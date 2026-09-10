"""Pinned, offline meow planning/scoring adapter. No engine server or transport runs."""
from __future__ import annotations

import argparse
import base64
from collections import Counter, defaultdict
import hashlib
import importlib
import importlib.metadata
import importlib.util
import json
from pathlib import Path
import sys

COMMIT = "fdb89c99852e0d5558551168835387b835265942"
MAX_INPUT = 2 * 1024 * 1024
MAX_OUTPUT = 4 * 1024 * 1024
MAX_SOURCE = 32 * 1024 * 1024


class AdapterError(Exception):
    pass


def strict_json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise AdapterError("invalid_input")
            result[key] = value
        return result

    def reject_constant(_):
        raise AdapterError("invalid_input")

    return json.loads(raw, object_pairs_hook=pairs, parse_constant=reject_constant)


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False).encode("utf-8")


def digest(value):
    return hashlib.sha256(value).hexdigest()


def read_bounded(path, maximum=MAX_SOURCE):
    with path.open("rb") as stream:
        raw = stream.read(maximum + 1)
    if len(raw) > maximum:
        raise AdapterError("size_limit")
    return raw


def verify_source(root, lock):
    root = root.resolve(strict=True)
    if lock.get("schema_version") != 1 or lock.get("engine_commit") != COMMIT or lock.get("engine_version") != "4.5.2":
        raise AdapterError("unsupported_engine")
    files = lock.get("files")
    if not isinstance(files, dict) or not files:
        raise AdapterError("invalid_lock")
    for name, expected in files.items():
        relative = Path(name)
        if relative.is_absolute() or ".." in relative.parts:
            raise AdapterError("invalid_lock")
        path = root / relative
        if not path.resolve(strict=True).is_relative_to(root) or path.is_symlink():
            raise AdapterError("source_mismatch")
        if digest(read_bounded(path)) != expected:
            raise AdapterError("source_mismatch")
    # Extra importable code cannot silently join a pinned Python package.
    for path in (root / "gpt56_vnext").rglob("*"):
        if path.is_symlink() or path.suffix in {".pyc", ".pyo", ".pyd", ".so"}:
            raise AdapterError("unverified_import")
        if path.suffix == ".py" and path.relative_to(root).as_posix() not in files:
            raise AdapterError("unverified_import")
    return root


class MeowAdapter:
    def __init__(self, root, lock, admitted=False):
        self.root = verify_source(root, lock)
        self.lock = lock
        if not admitted:
            raise AdapterError("engine_not_admitted")
        for name, version in lock["dependencies"].items():
            if importlib.metadata.version(name) != version:
                raise AdapterError("dependency_mismatch")
        if any(name == "gpt56_vnext" or name.startswith("gpt56_vnext.") for name in sys.modules):
            raise AdapterError("engine_already_loaded")
        sys.dont_write_bytecode = True
        spec = importlib.util.spec_from_file_location("gpt56_vnext", self.root / "gpt56_vnext" / "__init__.py",
            submodule_search_locations=[str(self.root / "gpt56_vnext")])
        module = importlib.util.module_from_spec(spec)
        sys.modules["gpt56_vnext"] = module
        spec.loader.exec_module(module)
        self.benchmark = importlib.import_module("gpt56_vnext.benchmark")
        self.detector = importlib.import_module("gpt56_vnext.detector")
        self.transport = importlib.import_module("gpt56_vnext.transport")
        self.normalizers = importlib.import_module("gpt56_vnext.normalizers")
        self.scoring = importlib.import_module("gpt56_vnext.probability_model")
        if self.scoring.SCORING_VERSION != lock["scoring_version"]:
            raise AdapterError("unsupported_engine")

    def validate_candidate(self, raw, manifest):
        if digest(raw) != manifest["sha256"]:
            raise AdapterError("source_mismatch")
        package = self.benchmark.load_package(raw)
        for key in ("id", "version", "mode", "content_sha256"):
            if package[key] != manifest[key]:
                raise AdapterError("benchmark_mismatch")
        if package["engine"]["scoring_version"] != self.scoring.SCORING_VERSION:
            raise AdapterError("unsupported_engine")
        cells = self.benchmark.cells_by_id(package)
        totals = {}
        for tier in self.benchmark.TIERS:
            if not self.detector.calibration_matches(package, tier):
                raise AdapterError("benchmark_recalibration_required")
            totals[tier] = sum(package["tiers"][tier]["counts"].values())
            if not 1 <= totals[tier] <= 300:
                raise AdapterError("plan_limit")
            for cell_id, count in package["tiers"][tier]["counts"].items():
                if count:
                    self.transport.build_payload(package["mode"], "compatibility-check", cells[cell_id])
        return {"engine_commit": COMMIT, "engine_lock_sha256": digest(canonical(self.lock)),
                "adapter_sha256": digest(read_bounded(Path(__file__))),
                "benchmark_sha256": manifest["sha256"], "content_sha256": manifest["content_sha256"],
                "planned_samples": totals, "calibration_valid": True}

    def package(self, identity, version):
        found = [b for b in self.lock["benchmarks"] if b["id"] == identity and b["version"] == version]
        if len(found) != 1:
            raise AdapterError("unknown_benchmark")
        manifest = found[0]
        raw = read_bounded(self.root / manifest["path"])
        if digest(raw) != manifest["sha256"]:
            raise AdapterError("source_mismatch")
        package = self.benchmark.load_package(raw)
        if package["content_sha256"] != manifest["content_sha256"] or package["mode"] != manifest["mode"]:
            raise AdapterError("benchmark_mismatch")
        return package, manifest

    def handle_release(self, request, catalog, reference):
        if digest(canonical(self.lock)) != reference.get("engine"):
            raise AdapterError("engine_mismatch")
        manifest, raw = catalog.resolve(reference)
        if request.get("benchmark_id") != manifest["id"] or request.get("benchmark_version") != manifest["version"]:
            raise AdapterError("benchmark_mismatch")
        self.validate_candidate(raw, manifest)
        return self.handle(request, approved_package=(self.benchmark.load_package(raw), manifest))

    def handle(self, request, *, approved_package=None):
        if not isinstance(request, dict):
            raise AdapterError("invalid_input")
        base_fields = {"operation", "benchmark_id", "benchmark_version", "tier", "claimed_model", "request_model"}
        operation = request.get("operation")
        expected = base_fields if operation == "plan" else base_fields | {"contract_hash", "contract_status", "samples"}
        if operation not in {"plan", "score"} or set(request) != expected:
            raise AdapterError("invalid_input")
        for key in base_fields:
            value = request[key]
            if not isinstance(value, str) or not value or len(value.encode("utf-8")) > 256 or value != value.strip() or any(ord(c) < 32 for c in value):
                raise AdapterError("invalid_input")
        package, manifest = approved_package if approved_package is not None else self.package(request["benchmark_id"], request["benchmark_version"])
        tier_name = request["tier"]
        if tier_name not in self.benchmark.TIERS:
            raise AdapterError("unsupported_tier")
        claimed = request["claimed_model"]
        model = request["request_model"]
        if claimed == "reference-only:other" or model == "reference-only:other" or claimed not in {m["id"] for m in package["models"]}:
            raise AdapterError("unsupported_model")
        tier = package["tiers"][tier_name]
        counts = tier["counts"]
        total = sum(counts.values())
        if not 1 <= total <= 300:
            raise AdapterError("plan_limit")
        cells = self.benchmark.cells_by_id(package)
        jobs = self.detector.build_single_jobs(package, tier_name, model)
        payloads = {cell_id: self.transport.build_payload(package["mode"], model, cells[cell_id]) for cell_id in sorted(counts) if counts[cell_id] > 0}
        contract = {"engine_commit": COMMIT, "benchmark_sha256": manifest["sha256"], "tier": tier_name,
                    "claimed_model": claimed, "mode": package["mode"], "sample_policy": "60-percent-v1", "counts": counts, "payloads": payloads}
        contract_hash = digest(canonical(contract))
        metadata = {"engine_commit": COMMIT, "engine_version": "4.5.2", "benchmark_id": manifest["id"],
                    "benchmark_version": manifest["version"], "benchmark_sha256": manifest["sha256"],
                    "content_sha256": manifest["content_sha256"], "contract_hash": contract_hash,
                    "scoring_version": self.scoring.SCORING_VERSION, "sample_policy_version": "60-percent-v1",
                    "mode": package["mode"], "tier": tier_name, "planned_samples": total}
        if operation == "plan":
            return {**metadata, "cells": [{"cell_id": identity, "count": counts[identity], "payload": payloads[identity],
                                          "payload_sha256": digest(canonical(payloads[identity]))} for identity in payloads],
                    "jobs": jobs}
        if request["contract_hash"] != contract_hash:
            raise AdapterError("contract_mismatch")
        if request["contract_status"] not in {"exact", "mutated", "unsupported", "unknown"}:
            raise AdapterError("invalid_input")
        samples = request["samples"]
        if not isinstance(samples, list) or len(samples) > total:
            raise AdapterError("sample_limit")
        observed = defaultdict(Counter)
        seen = set()
        for sample in samples:
            if not isinstance(sample, dict) or set(sample) != {"cell_id", "sample_index", "answer"}:
                raise AdapterError("invalid_sample")
            identity, index, answer = sample["cell_id"], sample["sample_index"], sample["answer"]
            if not isinstance(identity, str) or identity not in counts or type(index) is not int or not 0 <= index < counts[identity]:
                raise AdapterError("invalid_sample")
            if (identity, index) in seen or not isinstance(answer, str) or len(answer.encode("utf-8")) > 8192:
                raise AdapterError("invalid_sample")
            seen.add((identity, index))
            observed[identity][self.normalizers.normalize_answer(answer, cells[identity]["normalizer"])] += 1
        fingerprint = self.scoring.score_counts(package["fitted"], dict(observed), counts, tier["thresholds"],
            calibrated=self.detector.calibration_matches(package, tier_name), claimed_model=claimed, completion_ratio=.6)
        # Do not expose category strings: they can contain an upstream's answer or
        # echoed credential. The raw upstream verdict is distinct from publication.
        published = fingerprint["verdict"] if request["contract_status"] == "exact" else "not_evaluated"
        return {**metadata, "verdict": published, "engine_verdict": fingerprint["verdict"],
                "winner": fingerprint["model"] if published in {"match", "mismatch"} else None,
                "contract_status": request["contract_status"], "reasons": fingerprint["reasons"],
                "matches": fingerprint["matches"], "scores": fingerprint["scores"], "thresholds": fingerprint["thresholds"],
                "valid_samples": fingerprint["valid_samples"], "quality_status": fingerprint["quality_status"],
                "cells": [{"cell_id": identity, **{key: cell[key] for key in ("planned", "minimum", "completed", "valid")}}
                          for identity, cell in sorted(fingerprint["cells"].items())]}


def deny_network(event, _args):
    if event in {"socket.connect", "socket.connect_ex", "socket.getaddrinfo", "socket.bind", "socket.sendto"}:
        raise AdapterError("network_forbidden")


def validate_envelope(adapter, request):
    if not isinstance(request, dict) or set(request) != {"payload", "manifest", "engine_lock"}:
        raise AdapterError("invalid_input")
    if not isinstance(request["manifest"], dict) or set(request["manifest"]) != {"id", "version", "mode", "sha256", "content_sha256"}:
        raise AdapterError("invalid_input")
    try:
        raw = base64.b64decode(request["payload"], validate=True)
        lock_raw = base64.b64decode(request["engine_lock"], validate=True)
    except (ValueError, TypeError):
        raise AdapterError("invalid_input") from None
    if not 0 < len(raw) <= MAX_SOURCE or len(lock_raw) > 65536:
        raise AdapterError("size_limit")
    if strict_json(lock_raw) != adapter.lock:
        raise AdapterError("engine_mismatch")
    receipt = adapter.validate_candidate(raw, request["manifest"])
    receipt["engine_lock_sha256"] = digest(lock_raw)
    return receipt


def execute_envelope(adapter, envelope):
    if not isinstance(envelope, dict) or set(envelope) != {"payload", "manifest", "engine_lock", "request"}:
        raise AdapterError("invalid_input")
    validate_envelope(adapter, {k: envelope[k] for k in ("payload", "manifest", "engine_lock")})
    raw = base64.b64decode(envelope["payload"], validate=True)
    package = adapter.benchmark.load_package(raw)
    request = envelope["request"]
    if not isinstance(request, dict):
        raise AdapterError("invalid_input")
    if request.get("operation") == "decode":
        if set(request) != {"operation", "stream"}:
            raise AdapterError("invalid_input")
        stream = base64.b64decode(request["stream"], validate=True)
        if len(stream) > 1024 * 1024:
            raise AdapterError("size_limit")
        decoded = stream.decode("utf-8", errors="strict")
        result = adapter.transport.parse_stream(decoded, package["mode"], adapter.transport.SecretGuard())
        if len(result["answer"].encode("utf-8")) > 8192:
            raise AdapterError("size_limit")
        return {"answer": result["answer"], "usage": {k: result["usage"].get(k) for k in ("input_tokens", "output_tokens")}}
    if request.get("benchmark_id") != package["id"] or request.get("benchmark_version") != package["version"]:
        raise AdapterError("benchmark_mismatch")
    result = adapter.handle(request, approved_package=(package, envelope["manifest"]))
    if request["operation"] == "plan":
        for cell in result["cells"]:
            cell["payload_base64"] = base64.b64encode(canonical(cell["payload"])).decode("ascii")
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--engine-root", required=True, type=Path)
    parser.add_argument("--verify-only", action="store_true")
    parser.add_argument("--admitted", action="store_true")
    parser.add_argument("--validate-candidate", action="store_true")
    parser.add_argument("--execute-frozen", action="store_true")
    args = parser.parse_args()
    try:
        sys.addaudithook(deny_network)
        lock = strict_json(read_bounded(Path(__file__).with_name("meow.lock.json"), 65536))
        if args.verify_only:
            verify_source(args.engine_root, lock)
            result = {"verified": True, "engine_commit": COMMIT, "files": len(lock["files"])}
        else:
            maximum = 48 * 1024 * 1024 if args.validate_candidate or args.execute_frozen else MAX_INPUT
            raw = sys.stdin.buffer.read(maximum + 1)
            if len(raw) > maximum:
                raise AdapterError("size_limit")
            request = strict_json(raw)
            adapter = MeowAdapter(args.engine_root, lock, admitted=args.admitted)
            if args.validate_candidate and args.execute_frozen:
                raise AdapterError("invalid_input")
            result = execute_envelope(adapter, request) if args.execute_frozen else validate_envelope(adapter, request) if args.validate_candidate else adapter.handle(request)
        output = canonical(result)
        if len(output) > MAX_OUTPUT:
            raise AdapterError("size_limit")
        sys.stdout.buffer.write(output + b"\n")
        return 0
    except AdapterError as exc:
        sys.stdout.buffer.write(canonical({"error": str(exc)}) + b"\n")
        return 1
    except Exception:
        # No Python traceback, local path, input, endpoint or engine error text.
        sys.stdout.buffer.write(b'{"error":"adapter_failed"}\n')
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
