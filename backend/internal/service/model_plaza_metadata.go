package service

import (
	"regexp"
	"strconv"
	"strings"
)

// PlazaModelMetadata 模型广场展示用的模型元数据。
// JSON 字段与 frontend/src/api/modelPlaza.ts 的 PlazaModelMetadata 契约完全一致。
type PlazaModelMetadata struct {
	DisplayName     string   `json:"display_name"`
	Description     string   `json:"description"`
	Vendor          string   `json:"vendor"`
	ContextWindow   *int     `json:"context_window"`
	MaxOutputTokens *int     `json:"max_output_tokens"`
	Capabilities    []string `json:"capabilities"`
}

type plazaModelCatalogEntry struct {
	DisplayName     string
	DescriptionZH   string
	DescriptionEN   string
	Vendor          string
	ContextWindow   int
	MaxOutputTokens int
	Capabilities    []string
}

type plazaModelPrefixEntry struct {
	Prefix string
	Entry  plazaModelCatalogEntry
}

var (
	plazaDateDashSuffixRe    = regexp.MustCompile(`-\d{4}-\d{2}-\d{2}$`)
	plazaDateCompactSuffixRe = regexp.MustCompile(`-\d{8}$`)
	plazaBedrockVersionRe    = regexp.MustCompile(`-v\d+(?::\d+)?$`)
)

// plazaBuiltinModelCatalog 内置主流模型元信息精确表（key 为规范化后的小写模型名）。
var plazaBuiltinModelCatalog = map[string]plazaModelCatalogEntry{
	// OpenAI GPT-6 系列
	"gpt-6": {
		DisplayName: "GPT-6", Vendor: "openai", ContextWindow: 1_050_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 新一代旗舰多模态模型，面向复杂推理、工程开发与超长上下文任务",
		DescriptionEN: "OpenAI's next-generation flagship multimodal model for complex reasoning, engineering, and long-context tasks",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-6-astra": {
		DisplayName: "GPT-6 Astra", Vendor: "openai", ContextWindow: 1_050_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 顶级深度推理与复杂工程旗舰，适合高难度代码与多步规划",
		DescriptionEN: "OpenAI's top-tier flagship for deep reasoning, complex software engineering, and multi-step planning",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-6.2-astra": {
		DisplayName: "GPT-6.2 Astra", Vendor: "openai", ContextWindow: 1_050_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 旗舰推理升级版，兼顾顶尖代码能力与大上下文稳定性",
		DescriptionEN: "Upgraded OpenAI flagship combining top coding accuracy with long-context reliability",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-6.2-sol": {
		DisplayName: "GPT-6.2 Sol", Vendor: "openai", ContextWindow: 1_050_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 新一代均衡主力模型，在推理精度、代码工程、工具调用与 1.05M 长上下文间实现极佳平衡",
		DescriptionEN: "OpenAI's next-generation balanced workhorse uniting strong reasoning, coding, tool use, and a 1.05M context window",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-6-sol": {
		DisplayName: "GPT-6 Sol", Vendor: "openai", ContextWindow: 1_050_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 均衡主力模型，在推理质量、响应速度与成本间表现平衡",
		DescriptionEN: "Balanced OpenAI workhorse model combining strong reasoning, low latency, and efficiency",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-6.1-sol": {
		DisplayName: "GPT-6.1 Sol", Vendor: "openai", ContextWindow: 1_050_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 均衡主力升级版，适合日常开发、Agent 工具调用与多模态交互",
		DescriptionEN: "Upgraded balanced OpenAI model for daily engineering, agentic tool use, and multimodal workflows",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-6.1-terra": {
		DisplayName: "GPT-6.1 Terra", Vendor: "openai", ContextWindow: 1_050_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 高性价比主力模型，适合高频业务调用与长文档处理",
		DescriptionEN: "Cost-efficient OpenAI workhorse tailored for high-throughput workloads and long documents",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs"},
	},
	"gpt-6-luna": {
		DisplayName: "GPT-6 Luna", Vendor: "openai", ContextWindow: 1_050_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 轻量高速模型，极低延迟与成本，适合批量处理与轻量对话",
		DescriptionEN: "Fast, lightweight OpenAI model with ultra-low cost for high-volume and low-latency workloads",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs"},
	},

	// OpenAI GPT-5 / Codex 系列
	"gpt-5": {
		DisplayName: "GPT-5", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 旗舰统一模型，兼具深度思考、代码工程与多模态理解能力",
		DescriptionEN: "OpenAI flagship unified model excelling at deep reasoning, coding, and multimodal tasks",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-5-mini": {
		DisplayName: "GPT-5 Mini", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 128_000,
		DescriptionZH: "GPT-5 轻量高效版本，响应迅捷且成本友好，适合高频对话与工具调用",
		DescriptionEN: "Fast, cost-efficient GPT-5 variant suited for high-frequency chat and tool calling",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs"},
	},
	"gpt-5-nano": {
		DisplayName: "GPT-5 Nano", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 64_000,
		DescriptionZH: "GPT-5 超轻量低成本模型，适合分类抽取、摘要与高并发轻量任务",
		DescriptionEN: "Ultra-fast, lowest-cost GPT-5 tier built for classification, extraction, and high-concurrency tasks",
		Capabilities:  []string{"tools", "vision", "prompt_caching", "structured_outputs"},
	},
	"gpt-5-codex": {
		DisplayName: "GPT-5 Codex", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 面向软件工程与 Agent 编程专项优化的 Codex 模型",
		DescriptionEN: "OpenAI Codex model specialized for agentic software engineering and repository-scale coding",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"gpt-5.1": {
		DisplayName: "GPT-5.1", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 128_000,
		DescriptionZH: "GPT-5 迭代升级版，指令遵循与自适应推理更精准",
		DescriptionEN: "Enhanced GPT-5 release with stronger instruction following and adaptive reasoning",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-5.1-codex": {
		DisplayName: "GPT-5.1 Codex", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 128_000,
		DescriptionZH: "面向长程自主编程与重构任务优化的 GPT-5.1 Codex 模型",
		DescriptionEN: "GPT-5.1 Codex tuned for long-horizon autonomous coding and refactoring workflows",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"gpt-5.2": {
		DisplayName: "GPT-5.2", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 综合主力模型，强化复杂分析、代码生成与工具编排表现",
		DescriptionEN: "OpenAI general flagship with upgraded analysis, coding, and tool orchestration",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-5.2-codex": {
		DisplayName: "GPT-5.2 Codex", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 128_000,
		DescriptionZH: "面向端到端代码库改造与测试修复优化的 Codex 工程模型",
		DescriptionEN: "Codex engineering model optimized for end-to-end codebase changes and test verification",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"gpt-5.3-codex": {
		DisplayName: "GPT-5.3 Codex", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 128_000,
		DescriptionZH: "高自主度 Codex 编程主力，擅长大型项目排障与多文件协同修改",
		DescriptionEN: "High-autonomy Codex model built for complex debugging and multi-file engineering",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"gpt-5.4": {
		DisplayName: "GPT-5.4", Vendor: "openai", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 百万上下文旗舰模型，面向深度推理、代码工程与多模态工作流",
		DescriptionEN: "OpenAI 1M-context flagship built for deep reasoning, coding, and multimodal workflows",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-5.4-mini": {
		DisplayName: "GPT-5.4 Mini", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 128_000,
		DescriptionZH: "GPT-5.4 高速高性价比版本，适合高频开发助手与日常自动化",
		DescriptionEN: "Fast, cost-effective GPT-5.4 tier for high-throughput coding assistants and automation",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-5.4-nano": {
		DisplayName: "GPT-5.4 Nano", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 64_000,
		DescriptionZH: "GPT-5.4 极速轻量版本，延迟极低，适合结构化抽取与大流量任务",
		DescriptionEN: "Ultra-lightweight GPT-5.4 tier with minimal latency for extraction and high-volume routing",
		Capabilities:  []string{"tools", "vision", "prompt_caching", "structured_outputs"},
	},
	"gpt-5.5": {
		DisplayName: "GPT-5.5", Vendor: "openai", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "OpenAI 进阶旗舰模型，显著增强复杂编程、工具链与长程规划能力",
		DescriptionEN: "Advanced OpenAI flagship with stronger coding, tool chaining, and long-horizon planning",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-5.6-sol": {
		DisplayName: "GPT-5.6 Sol", Vendor: "openai", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "GPT-5.6 旗舰推理档，适合复杂系统设计与高精度代码工程",
		DescriptionEN: "Flagship GPT-5.6 reasoning tier for system architecture and high-precision engineering",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-5.6-terra": {
		DisplayName: "GPT-5.6 Terra", Vendor: "openai", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "GPT-5.6 均衡主力档，在能力、吞吐与成本之间取得理想平衡",
		DescriptionEN: "Balanced GPT-5.6 workhorse delivering strong performance across cost and throughput",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-5.6-luna": {
		DisplayName: "GPT-5.6 Luna", Vendor: "openai", ContextWindow: 400_000, MaxOutputTokens: 64_000,
		DescriptionZH: "GPT-5.6 极速轻量档，低成本高吞吐，适合日常问答与批处理",
		DescriptionEN: "Lightweight, high-speed GPT-5.6 tier optimized for everyday chat and batch workloads",
		Capabilities:  []string{"tools", "vision", "prompt_caching", "structured_outputs"},
	},

	// OpenAI GPT-4o / GPT-4.1 / o-series / Image
	"gpt-4.1": {
		DisplayName: "GPT-4.1", Vendor: "openai", ContextWindow: 1_047_576, MaxOutputTokens: 32_768,
		DescriptionZH: "OpenAI 1M 上下文非推理主力模型，指令遵循与前端代码表现出色",
		DescriptionEN: "OpenAI 1M-context non-reasoning workhorse with crisp instruction following and coding",
		Capabilities:  []string{"tools", "vision", "prompt_caching", "long_context", "structured_outputs", "code"},
	},
	"gpt-4.1-mini": {
		DisplayName: "GPT-4.1 Mini", Vendor: "openai", ContextWindow: 1_047_576, MaxOutputTokens: 32_768,
		DescriptionZH: "GPT-4.1 轻量版，兼顾 1M 长上下文、低延迟与优异性价比",
		DescriptionEN: "Compact GPT-4.1 model combining a 1M context window with low latency and cost",
		Capabilities:  []string{"tools", "vision", "prompt_caching", "long_context", "structured_outputs"},
	},
	"gpt-4.1-nano": {
		DisplayName: "GPT-4.1 Nano", Vendor: "openai", ContextWindow: 1_047_576, MaxOutputTokens: 32_768,
		DescriptionZH: "GPT-4.1 极速轻量模型，专为低延迟分类、路由与自动补全打造",
		DescriptionEN: "Fastest GPT-4.1 variant engineered for classification, routing, and autocompletion",
		Capabilities:  []string{"tools", "vision", "prompt_caching", "structured_outputs"},
	},
	"gpt-4o": {
		DisplayName: "GPT-4o", Vendor: "openai", ContextWindow: 128_000, MaxOutputTokens: 16_384,
		DescriptionZH: "OpenAI 经典全能多模态模型，兼顾文本、视觉理解与稳定响应速度",
		DescriptionEN: "OpenAI's versatile omni multimodal model balancing text, vision, and responsive latency",
		Capabilities:  []string{"tools", "vision", "prompt_caching", "structured_outputs"},
	},
	"gpt-4o-mini": {
		DisplayName: "GPT-4o Mini", Vendor: "openai", ContextWindow: 128_000, MaxOutputTokens: 16_384,
		DescriptionZH: "高性价比轻量多模态模型，适合高频日常对话与轻量级视觉任务",
		DescriptionEN: "Affordable, fast multimodal model for high-frequency chat and lightweight vision tasks",
		Capabilities:  []string{"tools", "vision", "prompt_caching", "structured_outputs"},
	},
	"o3": {
		DisplayName: "OpenAI o3", Vendor: "openai", ContextWindow: 200_000, MaxOutputTokens: 100_000,
		DescriptionZH: "OpenAI 深度推理模型，擅长数学推导、算法设计与复杂逻辑排障",
		DescriptionEN: "OpenAI deep reasoning model built for mathematics, algorithms, and complex problem solving",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "structured_outputs", "code"},
	},
	"o3-pro": {
		DisplayName: "OpenAI o3-pro", Vendor: "openai", ContextWindow: 200_000, MaxOutputTokens: 100_000,
		DescriptionZH: "o3 高算力深度思考版本，为高难度科研与系统架构提供最高可靠性",
		DescriptionEN: "Extended-compute o3 tier delivering peak reliability for demanding research and architecture",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "structured_outputs", "code"},
	},
	"o3-mini": {
		DisplayName: "OpenAI o3-mini", Vendor: "openai", ContextWindow: 200_000, MaxOutputTokens: 100_000,
		DescriptionZH: "轻量高速 STEM 推理模型，低成本完成代码与数理逻辑任务",
		DescriptionEN: "Fast, cost-efficient STEM reasoning model for coding and quantitative analysis",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "structured_outputs", "code"},
	},
	"o4-mini": {
		DisplayName: "OpenAI o4-mini", Vendor: "openai", ContextWindow: 200_000, MaxOutputTokens: 100_000,
		DescriptionZH: "OpenAI 新一代轻量视觉推理模型，高速且擅长数学与编程",
		DescriptionEN: "Next-generation compact multimodal reasoning model excelling at math and coding",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "structured_outputs", "code"},
	},
	"gpt-image-1": {
		DisplayName: "GPT Image 1", Vendor: "openai",
		DescriptionZH: "OpenAI 原生多模态图像生成与编辑模型，擅长精准文字排版与细节还原",
		DescriptionEN: "OpenAI native image generation and editing model with strong text rendering and fidelity",
		Capabilities:  []string{"image", "vision"},
	},
	"gpt-image-1.5": {
		DisplayName: "GPT Image 1.5", Vendor: "openai",
		DescriptionZH: "OpenAI 进阶图像生成与编辑模型，构图控制与指令遵循更精准",
		DescriptionEN: "Upgraded OpenAI image generator with sharper composition control and prompt adherence",
		Capabilities:  []string{"image", "vision"},
	},
	"gpt-image-2": {
		DisplayName: "GPT Image 2", Vendor: "openai",
		DescriptionZH: "OpenAI 新一代高保真图像创作与编辑模型，支持多分辨率输出",
		DescriptionEN: "Next-generation OpenAI image generation and editing model with multi-resolution output",
		Capabilities:  []string{"image", "vision"},
	},

	// Anthropic Claude 系列
	"claude-3-7-sonnet": {
		DisplayName: "Claude 3.7 Sonnet", Vendor: "anthropic", ContextWindow: 200_000, MaxOutputTokens: 64_000,
		DescriptionZH: "Anthropic 首个混合推理 Sonnet 模型，代码、写作与工具调用均衡出色",
		DescriptionEN: "Anthropic's hybrid-reasoning Sonnet model balancing coding, writing, and tool use",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "code"},
	},
	"claude-sonnet-4": {
		DisplayName: "Claude Sonnet 4", Vendor: "anthropic", ContextWindow: 200_000, MaxOutputTokens: 64_000,
		DescriptionZH: "Anthropic 均衡主力模型，在软件工程、Agent 执行与提示词缓存上表现优异",
		DescriptionEN: "Balanced Anthropic workhorse with strong software engineering, agent execution, and prompt caching",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "code"},
	},
	"claude-opus-4": {
		DisplayName: "Claude Opus 4", Vendor: "anthropic", ContextWindow: 200_000, MaxOutputTokens: 32_000,
		DescriptionZH: "Anthropic 旗舰编程与深度推理模型，擅长长程自主代码工程",
		DescriptionEN: "Anthropic flagship model for deep reasoning and sustained autonomous coding",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "code"},
	},
	"claude-opus-4-1": {
		DisplayName: "Claude Opus 4.1", Vendor: "anthropic", ContextWindow: 200_000, MaxOutputTokens: 32_000,
		DescriptionZH: "Claude Opus 4 强化版，进一步提升多文件代码重构与复杂推理精度",
		DescriptionEN: "Upgraded Claude Opus 4 with higher accuracy on multi-file refactoring and complex reasoning",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "code"},
	},
	"claude-sonnet-4-5": {
		DisplayName: "Claude Sonnet 4.5", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 64_000,
		DescriptionZH: "Anthropic 编程与 Agent 黄金主力，支持提示词缓存与 1M 长上下文",
		DescriptionEN: "Anthropic's premier coding and agentic workhorse with prompt caching and 1M long context",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-opus-4-5": {
		DisplayName: "Claude Opus 4.5", Vendor: "anthropic", ContextWindow: 200_000, MaxOutputTokens: 64_000,
		DescriptionZH: "Anthropic 旗舰智能模型，复杂架构设计与深度代码审查能力出众",
		DescriptionEN: "Anthropic flagship model excelling at complex architecture design and deep code review",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "code"},
	},
	"claude-haiku-4-5": {
		DisplayName: "Claude Haiku 4.5", Vendor: "anthropic", ContextWindow: 200_000, MaxOutputTokens: 64_000,
		DescriptionZH: "Anthropic 极速轻量模型，近实时首字响应且具备出色编程与工具能力",
		DescriptionEN: "Anthropic's fastest compact model combining near-instant latency with strong coding and tool use",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "code"},
	},
	"claude-sonnet-4-6": {
		DisplayName: "Claude Sonnet 4.6", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 64_000,
		DescriptionZH: "Anthropic 新一代主力 Sonnet，全面强化代码生成、工具协作与缓存效率",
		DescriptionEN: "Next-generation Claude Sonnet with upgraded coding, tool collaboration, and caching efficiency",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-opus-4-6": {
		DisplayName: "Claude Opus 4.6", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "Anthropic 旗舰深度推理模型，支持 1M 上下文与超强长程 Agent 编码",
		DescriptionEN: "Flagship Anthropic reasoning model with 1M context and top-tier long-horizon agentic coding",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-opus-4-7": {
		DisplayName: "Claude Opus 4.7", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "Anthropic 旗舰进阶版，专注高精度软件工程与高难逻辑推理",
		DescriptionEN: "Advanced Anthropic Opus flagship focused on high-precision engineering and complex logic",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-opus-4-8": {
		DisplayName: "Claude Opus 4.8", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "Anthropic 顶级旗舰模型，为大型仓库级开发与复杂决策而生",
		DescriptionEN: "Top-tier Anthropic flagship engineered for repository-scale development and complex decisions",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-sonnet-5": {
		DisplayName: "Claude Sonnet 5", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "Claude 5 代均衡主力模型，兼顾极高编码质量、长上下文与响应效率",
		DescriptionEN: "Claude 5 balanced workhorse pairing high coding accuracy with 1M context and speed",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-sonnet-5-5": {
		DisplayName: "Claude Sonnet 5.5", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "Anthropic 高效编程与 Agent 主力，极佳的提示词缓存收益与长程稳定性",
		DescriptionEN: "High-efficiency Anthropic coding and agent workhorse with outstanding prompt cache savings",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-opus-5": {
		DisplayName: "Claude Opus 5", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "Claude 5 代顶级旗舰模型，擅长最复杂的系统架构与前沿推理任务",
		DescriptionEN: "Claude 5 flagship model built for the hardest architecture, research, and coding tasks",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-opus-5-5": {
		DisplayName: "Claude Opus 5.5", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "Anthropic 顶级深度思考与软件工程旗舰，支持多档推理力度",
		DescriptionEN: "Premier Anthropic reasoning and software engineering flagship with multi-level thinking effort",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-haiku-5-5": {
		DisplayName: "Claude Haiku 5.5", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "Claude 5 代超快轻量模型，支持 1M 上下文与自适应思考，极低单价与毫秒级首字响应",
		DescriptionEN: "Ultra-fast Claude 5 compact model with 1M context, adaptive thinking, minimal cost, and rapid time-to-first-token",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-fable-5-2": {
		DisplayName: "Claude Fable 5.2", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "Anthropic 前沿创作与深度推理旗舰，兼顾超长上下文与高缓存命中",
		DescriptionEN: "Frontier Anthropic model for deep reasoning, creative work, and long-context engineering",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"claude-mythos-5-2": {
		DisplayName: "Claude Mythos 5.2", Vendor: "anthropic", ContextWindow: 1_000_000, MaxOutputTokens: 128_000,
		DescriptionZH: "Anthropic 前沿高阶推理旗舰，面向复杂研究、安全分析与代码工程",
		DescriptionEN: "Frontier Anthropic flagship designed for advanced research, analysis, and software engineering",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},

	// Google Gemini 系列
	"gemini-2.5-pro": {
		DisplayName: "Gemini 2.5 Pro", Vendor: "google", ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		DescriptionZH: "Google 原生多模态推理旗舰，1M 上下文轻松处理长代码库与音视频文档",
		DescriptionEN: "Google's flagship multimodal reasoning model with a 1M context window for code, audio, and video",
		Capabilities:  []string{"reasoning", "tools", "vision", "audio", "prompt_caching", "long_context", "web_search", "structured_outputs", "code"},
	},
	"gemini-2.5-flash": {
		DisplayName: "Gemini 2.5 Flash", Vendor: "google", ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		DescriptionZH: "Google 高速高性价比思考模型，支持 1M 上下文与可控思考预算",
		DescriptionEN: "Fast, cost-effective Google thinking model with 1M context and controllable reasoning budget",
		Capabilities:  []string{"reasoning", "tools", "vision", "audio", "prompt_caching", "long_context", "web_search", "structured_outputs"},
	},
	"gemini-2.5-flash-lite": {
		DisplayName: "Gemini 2.5 Flash-Lite", Vendor: "google", ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		DescriptionZH: "Google 超低成本极速模型，适合大规模批处理、翻译与实时分类",
		DescriptionEN: "Ultra-low-cost, high-throughput Google model built for batch processing and real-time classification",
		Capabilities:  []string{"tools", "vision", "audio", "prompt_caching", "long_context", "structured_outputs"},
	},
	"gemini-2.5-flash-image": {
		DisplayName: "Gemini 2.5 Flash Image", Vendor: "google", ContextWindow: 65_536, MaxOutputTokens: 32_768,
		DescriptionZH: "Google 原生多模态生图与图像编辑模型，支持对话式多轮改图",
		DescriptionEN: "Google native multimodal image generation and conversational editing model",
		Capabilities:  []string{"image", "vision"},
	},
	"gemini-3-pro": {
		DisplayName: "Gemini 3 Pro", Vendor: "google", ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		DescriptionZH: "Google 新一代多模态推理旗舰，强化复杂代码、前端生成与跨模态理解",
		DescriptionEN: "Next-generation Google multimodal flagship with elevated coding, UI generation, and reasoning",
		Capabilities:  []string{"reasoning", "tools", "vision", "audio", "prompt_caching", "long_context", "web_search", "structured_outputs", "code"},
	},
	"gemini-3-flash": {
		DisplayName: "Gemini 3 Flash", Vendor: "google", ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		DescriptionZH: "Google 3 代高速主力模型，极速响应同时保留强推理与工具调用能力",
		DescriptionEN: "High-speed Gemini 3 workhorse combining rapid latency with strong reasoning and tool use",
		Capabilities:  []string{"reasoning", "tools", "vision", "audio", "prompt_caching", "long_context", "web_search", "structured_outputs"},
	},
	"gemini-3.1-pro": {
		DisplayName: "Gemini 3.1 Pro", Vendor: "google", ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		DescriptionZH: "Google Gemini 3.1 旗舰版，大幅提升复杂工程推理与 Agent 工具可靠性",
		DescriptionEN: "Google Gemini 3.1 flagship delivering stronger engineering reasoning and agentic reliability",
		Capabilities:  []string{"reasoning", "tools", "vision", "audio", "prompt_caching", "long_context", "web_search", "structured_outputs", "code"},
	},
	"gemini-3.1-flash-image": {
		DisplayName: "Gemini 3.1 Flash Image", Vendor: "google", ContextWindow: 65_536, MaxOutputTokens: 32_768,
		DescriptionZH: "Google 3.1 代高速图像创作与编辑模型，支持高分辨率与精准细节控制",
		DescriptionEN: "Fast Gemini 3.1 image generation and editing model supporting high-resolution outputs",
		Capabilities:  []string{"image", "vision"},
	},
	"gemini-4-neon": {
		DisplayName: "Gemini 4 Neon", Vendor: "google", ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		DescriptionZH: "Google Gemini 4 新一代旗舰多模态推理模型，支持多档思考力度、1M 超长上下文与复杂代码工程",
		DescriptionEN: "Google Gemini 4 next-generation flagship multimodal reasoning model with tiered thinking effort, 1M context, and advanced coding",
		Capabilities:  []string{"reasoning", "tools", "vision", "audio", "prompt_caching", "long_context", "web_search", "structured_outputs", "code"},
	},
	"gemini-4-hydrogen": {
		DisplayName: "Gemini 4 Hydrogen", Vendor: "google", ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		DescriptionZH: "Google Gemini 4 高速高性价比思考模型，支持多档推理力度与 1M 长上下文多模态处理",
		DescriptionEN: "Fast, cost-efficient Google Gemini 4 thinking model with tiered reasoning effort and 1M multimodal context",
		Capabilities:  []string{"reasoning", "tools", "vision", "audio", "prompt_caching", "long_context", "web_search", "structured_outputs", "code"},
	},
	"gemini-4-carbon": {
		DisplayName: "Gemini 4 Carbon", Vendor: "google", ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		DescriptionZH: "Google Gemini 4 旗舰多模态模型，面向高难度推理与超长上下文工程",
		DescriptionEN: "Google Gemini 4 flagship multimodal model for frontier reasoning and long-context engineering",
		Capabilities:  []string{"reasoning", "tools", "vision", "audio", "prompt_caching", "long_context", "web_search", "structured_outputs", "code"},
	},
	"gemini-4-argon": {
		DisplayName: "Gemini 4 Argon", Vendor: "google", ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		DescriptionZH: "Google Gemini 4 敏捷推理主力，支持多档思考力度与高速响应",
		DescriptionEN: "Agile Gemini 4 workhorse offering tiered reasoning effort and fast response times",
		Capabilities:  []string{"reasoning", "tools", "vision", "audio", "prompt_caching", "long_context", "web_search", "structured_outputs", "code"},
	},

	// xAI Grok 系列
	"grok-3": {
		DisplayName: "Grok 3", Vendor: "xai", ContextWindow: 131_072, MaxOutputTokens: 32_768,
		DescriptionZH: "xAI 旗舰对话与推理模型，擅长实时信息分析、代码与逻辑推理",
		DescriptionEN: "xAI flagship model excelling at real-time analysis, coding, and reasoning",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "web_search", "structured_outputs", "code"},
	},
	"grok-3-mini": {
		DisplayName: "Grok 3 Mini", Vendor: "xai", ContextWindow: 131_072, MaxOutputTokens: 32_768,
		DescriptionZH: "xAI 轻量推理模型，响应迅速且性价比极高",
		DescriptionEN: "Compact, high-value xAI reasoning model built for fast math and logic tasks",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "structured_outputs"},
	},
	"grok-4": {
		DisplayName: "Grok 4", Vendor: "xai", ContextWindow: 256_000, MaxOutputTokens: 64_000,
		DescriptionZH: "xAI 新一代旗舰多模态推理模型，支持实时搜索与高阶代码分析",
		DescriptionEN: "Next-generation xAI multimodal flagship with live search and advanced coding reasoning",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "web_search", "structured_outputs", "code"},
	},
	"grok-4-fast": {
		DisplayName: "Grok 4 Fast", Vendor: "xai", ContextWindow: 2_000_000, MaxOutputTokens: 64_000,
		DescriptionZH: "xAI 超长上下文极速模型，支持 2M 窗口与高吞吐低延迟推理",
		DescriptionEN: "High-speed xAI model featuring a 2M context window and low-latency reasoning",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "web_search", "structured_outputs"},
	},
	"grok-4.5": {
		DisplayName: "Grok 4.5", Vendor: "xai", ContextWindow: 256_000, MaxOutputTokens: 64_000,
		DescriptionZH: "xAI 进阶旗舰推理模型，深度强化代码工程与多步工具编排",
		DescriptionEN: "Upgraded xAI flagship strengthening software engineering and multi-step tool use",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "web_search", "structured_outputs", "code"},
	},
	"grok-4.6": {
		DisplayName: "Grok 4.6", Vendor: "xai", ContextWindow: 500_000, MaxOutputTokens: 64_000,
		DescriptionZH: "xAI 新一代主力推理与编程模型，支持 500K 上下文、实时搜索与高阶工具调用",
		DescriptionEN: "xAI's flagship reasoning and coding model featuring a 500K context window, live search, and tool use",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "web_search", "structured_outputs", "code"},
	},
	"grok-4.7": {
		DisplayName: "Grok 4.7", Vendor: "xai", ContextWindow: 500_000, MaxOutputTokens: 64_000,
		DescriptionZH: "xAI 最新旗舰模型，兼具强劲深度思考、联网检索与代码生成实力",
		DescriptionEN: "Latest xAI flagship combining deep reasoning, live web search, and code generation",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "web_search", "structured_outputs", "code"},
	},

	// DeepSeek 系列
	"deepseek-chat": {
		DisplayName: "DeepSeek Chat (V3)", Vendor: "deepseek", ContextWindow: 128_000, MaxOutputTokens: 8_192,
		DescriptionZH: "DeepSeek V3 通用对话与代码主力，超高性价比并原生支持磁盘缓存命中",
		DescriptionEN: "DeepSeek V3 general chat and coding workhorse with exceptional cost efficiency and prompt caching",
		Capabilities:  []string{"tools", "prompt_caching", "structured_outputs", "code"},
	},
	"deepseek-reasoner": {
		DisplayName: "DeepSeek Reasoner (R1)", Vendor: "deepseek", ContextWindow: 128_000, MaxOutputTokens: 64_000,
		DescriptionZH: "DeepSeek 深度思考推理模型，擅长复杂数学推导、算法竞赛与代码排障",
		DescriptionEN: "DeepSeek reasoning model excelling at mathematics, algorithms, and deep debugging",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "code"},
	},
	"deepseek-v3": {
		DisplayName: "DeepSeek V3", Vendor: "deepseek", ContextWindow: 128_000, MaxOutputTokens: 8_192,
		DescriptionZH: "DeepSeek V3 开源旗舰通用模型，中英文写作与编程性价比突出",
		DescriptionEN: "DeepSeek V3 flagship general-purpose model with strong coding and bilingual value",
		Capabilities:  []string{"tools", "prompt_caching", "structured_outputs", "code"},
	},
	"deepseek-r1": {
		DisplayName: "DeepSeek R1", Vendor: "deepseek", ContextWindow: 128_000, MaxOutputTokens: 64_000,
		DescriptionZH: "DeepSeek R1 开源推理旗舰，透明展示思维链，数理逻辑能力出众",
		DescriptionEN: "DeepSeek R1 open-weight reasoning flagship with transparent chain-of-thought for math and code",
		Capabilities:  []string{"reasoning", "prompt_caching", "code"},
	},

	// 国产主流模型（Kimi / 智谱 GLM / MiniMax / Qwen）
	"kimi-k2": {
		DisplayName: "Kimi K2", Vendor: "moonshot", ContextWindow: 256_000, MaxOutputTokens: 32_768,
		DescriptionZH: "Moonshot 万亿参数 MoE 模型，专注 Agent 工具调用、长文本与代码工程",
		DescriptionEN: "Moonshot trillion-parameter MoE model tuned for agentic tool use, long context, and coding",
		Capabilities:  []string{"tools", "prompt_caching", "long_context", "code"},
	},
	"kimi-k2.5": {
		DisplayName: "Kimi K2.5", Vendor: "moonshot", ContextWindow: 256_000, MaxOutputTokens: 32_768,
		DescriptionZH: "Moonshot 原生多模态推理与编程主力，兼具视觉理解与长程 Agent 能力",
		DescriptionEN: "Moonshot native multimodal reasoning and coding model with vision and long-horizon agentic capability",
		Capabilities:  []string{"reasoning", "tools", "vision", "prompt_caching", "long_context", "code"},
	},
	"glm-4.5": {
		DisplayName: "GLM-4.5", Vendor: "zhipu", ContextWindow: 128_000, MaxOutputTokens: 64_000,
		DescriptionZH: "智谱混合推理主力模型，原生面向 Agent 任务、代码生成与工具调用优化",
		DescriptionEN: "Zhipu hybrid-reasoning flagship engineered for agentic workflows, coding, and tool calling",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "code"},
	},
	"glm-4.6": {
		DisplayName: "GLM-4.6", Vendor: "zhipu", ContextWindow: 200_000, MaxOutputTokens: 64_000,
		DescriptionZH: "智谱新一代编程与长上下文主力模型，复杂代码任务表现稳健",
		DescriptionEN: "Zhipu's next-generation coding and long-context model for reliable software engineering",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "long_context", "code"},
	},
	"glm-4.7": {
		DisplayName: "GLM-4.7", Vendor: "zhipu", ContextWindow: 200_000, MaxOutputTokens: 64_000,
		DescriptionZH: "智谱进阶代码与推理旗舰，适合多步骤开发与高精度中文交互",
		DescriptionEN: "Upgraded Zhipu flagship for multi-step coding, deep reasoning, and Chinese-English tasks",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "long_context", "code"},
	},
	"glm-5": {
		DisplayName: "GLM-5", Vendor: "zhipu", ContextWindow: 200_000, MaxOutputTokens: 64_000,
		DescriptionZH: "智谱新一代全能旗舰模型，在深度推理、代码工程与多模态协作上全面升级",
		DescriptionEN: "Zhipu's flagship GLM-5 release upgrading deep reasoning, coding, and agentic collaboration",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "long_context", "code"},
	},
	"minimax-m2": {
		DisplayName: "MiniMax M2", Vendor: "minimax", ContextWindow: 200_000, MaxOutputTokens: 64_000,
		DescriptionZH: "MiniMax 面向编程与 Agent 工具链优化的高速高性价比推理模型",
		DescriptionEN: "Fast, cost-efficient MiniMax reasoning model tailored for coding and agentic toolchains",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "long_context", "code"},
	},
	"minimax-m2.5": {
		DisplayName: "MiniMax M2.5", Vendor: "minimax", ContextWindow: 200_000, MaxOutputTokens: 64_000,
		DescriptionZH: "MiniMax 旗舰编程与 Agent 模型，长上下文代码生成与吞吐表现优异",
		DescriptionEN: "MiniMax flagship coding and agent model with strong long-context throughput",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "long_context", "code"},
	},
	"qwen3-coder": {
		DisplayName: "Qwen3 Coder", Vendor: "qwen", ContextWindow: 256_000, MaxOutputTokens: 64_000,
		DescriptionZH: "通义千问代码专精大模型，面向仓库级代码理解与自主编程优化",
		DescriptionEN: "Alibaba Qwen coding specialist built for repository-level understanding and agentic programming",
		Capabilities:  []string{"tools", "prompt_caching", "long_context", "code"},
	},
	"qwen3-max": {
		DisplayName: "Qwen3 Max", Vendor: "qwen", ContextWindow: 256_000, MaxOutputTokens: 64_000,
		DescriptionZH: "通义千问旗舰综合模型，擅长复杂中文写作、逻辑推理与工具调用",
		DescriptionEN: "Flagship Alibaba Qwen model delivering strong reasoning, bilingual writing, and tool use",
		Capabilities:  []string{"reasoning", "tools", "prompt_caching", "long_context", "structured_outputs"},
	},
}

// plazaBuiltinPrefixRules 按前缀降序匹配的家族回落规则（处理 -high/-low/-thinking/-preview/-max 等变体）。
var plazaBuiltinPrefixRules = []plazaModelPrefixEntry{
	{Prefix: "gpt-6.2-astra", Entry: plazaBuiltinModelCatalog["gpt-6.2-astra"]},
	{Prefix: "gpt-6.2-sol", Entry: plazaBuiltinModelCatalog["gpt-6.2-sol"]},
	{Prefix: "gpt-6.1-sol", Entry: plazaBuiltinModelCatalog["gpt-6.1-sol"]},
	{Prefix: "gpt-6.1-terra", Entry: plazaBuiltinModelCatalog["gpt-6.1-terra"]},
	{Prefix: "gpt-6-astra", Entry: plazaBuiltinModelCatalog["gpt-6-astra"]},
	{Prefix: "gpt-6-sol", Entry: plazaBuiltinModelCatalog["gpt-6-sol"]},
	{Prefix: "gpt-6-luna", Entry: plazaBuiltinModelCatalog["gpt-6-luna"]},
	{Prefix: "gpt-6", Entry: plazaBuiltinModelCatalog["gpt-6"]},
	{Prefix: "gpt-5.6-sol", Entry: plazaBuiltinModelCatalog["gpt-5.6-sol"]},
	{Prefix: "gpt-5.6-terra", Entry: plazaBuiltinModelCatalog["gpt-5.6-terra"]},
	{Prefix: "gpt-5.6-luna", Entry: plazaBuiltinModelCatalog["gpt-5.6-luna"]},
	{Prefix: "gpt-5.5", Entry: plazaBuiltinModelCatalog["gpt-5.5"]},
	{Prefix: "gpt-5.4-mini", Entry: plazaBuiltinModelCatalog["gpt-5.4-mini"]},
	{Prefix: "gpt-5.4-nano", Entry: plazaBuiltinModelCatalog["gpt-5.4-nano"]},
	{Prefix: "gpt-5.4", Entry: plazaBuiltinModelCatalog["gpt-5.4"]},
	{Prefix: "gpt-5.3-codex", Entry: plazaBuiltinModelCatalog["gpt-5.3-codex"]},
	{Prefix: "gpt-5.2-codex", Entry: plazaBuiltinModelCatalog["gpt-5.2-codex"]},
	{Prefix: "gpt-5.2", Entry: plazaBuiltinModelCatalog["gpt-5.2"]},
	{Prefix: "gpt-5.1-codex", Entry: plazaBuiltinModelCatalog["gpt-5.1-codex"]},
	{Prefix: "gpt-5.1", Entry: plazaBuiltinModelCatalog["gpt-5.1"]},
	{Prefix: "gpt-5-codex", Entry: plazaBuiltinModelCatalog["gpt-5-codex"]},
	{Prefix: "gpt-5-mini", Entry: plazaBuiltinModelCatalog["gpt-5-mini"]},
	{Prefix: "gpt-5-nano", Entry: plazaBuiltinModelCatalog["gpt-5-nano"]},
	{Prefix: "gpt-5", Entry: plazaBuiltinModelCatalog["gpt-5"]},
	{Prefix: "gpt-4.1-mini", Entry: plazaBuiltinModelCatalog["gpt-4.1-mini"]},
	{Prefix: "gpt-4.1-nano", Entry: plazaBuiltinModelCatalog["gpt-4.1-nano"]},
	{Prefix: "gpt-4.1", Entry: plazaBuiltinModelCatalog["gpt-4.1"]},
	{Prefix: "gpt-4o-mini", Entry: plazaBuiltinModelCatalog["gpt-4o-mini"]},
	{Prefix: "gpt-4o", Entry: plazaBuiltinModelCatalog["gpt-4o"]},
	{Prefix: "gpt-image-2", Entry: plazaBuiltinModelCatalog["gpt-image-2"]},
	{Prefix: "gpt-image-1.5", Entry: plazaBuiltinModelCatalog["gpt-image-1.5"]},
	{Prefix: "gpt-image-1", Entry: plazaBuiltinModelCatalog["gpt-image-1"]},
	{Prefix: "o4-mini", Entry: plazaBuiltinModelCatalog["o4-mini"]},
	{Prefix: "o3-pro", Entry: plazaBuiltinModelCatalog["o3-pro"]},
	{Prefix: "o3-mini", Entry: plazaBuiltinModelCatalog["o3-mini"]},
	{Prefix: "o3", Entry: plazaBuiltinModelCatalog["o3"]},
	{Prefix: "claude-fable-5-2", Entry: plazaBuiltinModelCatalog["claude-fable-5-2"]},
	{Prefix: "claude-fable-5", Entry: plazaBuiltinModelCatalog["claude-fable-5-2"]},
	{Prefix: "claude-mythos-5-2", Entry: plazaBuiltinModelCatalog["claude-mythos-5-2"]},
	{Prefix: "claude-opus-5-5", Entry: plazaBuiltinModelCatalog["claude-opus-5-5"]},
	{Prefix: "claude-opus-5", Entry: plazaBuiltinModelCatalog["claude-opus-5"]},
	{Prefix: "claude-sonnet-5-5", Entry: plazaBuiltinModelCatalog["claude-sonnet-5-5"]},
	{Prefix: "claude-sonnet-5", Entry: plazaBuiltinModelCatalog["claude-sonnet-5"]},
	{Prefix: "claude-haiku-5-5", Entry: plazaBuiltinModelCatalog["claude-haiku-5-5"]},
	{Prefix: "claude-opus-4-8", Entry: plazaBuiltinModelCatalog["claude-opus-4-8"]},
	{Prefix: "claude-opus-4-7", Entry: plazaBuiltinModelCatalog["claude-opus-4-7"]},
	{Prefix: "claude-opus-4-6", Entry: plazaBuiltinModelCatalog["claude-opus-4-6"]},
	{Prefix: "claude-opus-4-5", Entry: plazaBuiltinModelCatalog["claude-opus-4-5"]},
	{Prefix: "claude-opus-4-1", Entry: plazaBuiltinModelCatalog["claude-opus-4-1"]},
	{Prefix: "claude-opus-4", Entry: plazaBuiltinModelCatalog["claude-opus-4"]},
	{Prefix: "claude-sonnet-4-6", Entry: plazaBuiltinModelCatalog["claude-sonnet-4-6"]},
	{Prefix: "claude-sonnet-4-5", Entry: plazaBuiltinModelCatalog["claude-sonnet-4-5"]},
	{Prefix: "claude-sonnet-4", Entry: plazaBuiltinModelCatalog["claude-sonnet-4"]},
	{Prefix: "claude-haiku-4-5", Entry: plazaBuiltinModelCatalog["claude-haiku-4-5"]},
	{Prefix: "claude-3-7-sonnet", Entry: plazaBuiltinModelCatalog["claude-3-7-sonnet"]},
	{Prefix: "claude-3.7-sonnet", Entry: plazaBuiltinModelCatalog["claude-3-7-sonnet"]},
	{Prefix: "gemini-4-neon", Entry: plazaBuiltinModelCatalog["gemini-4-neon"]},
	{Prefix: "gemini-4-hydrogen", Entry: plazaBuiltinModelCatalog["gemini-4-hydrogen"]},
	{Prefix: "gemini-4-carbon", Entry: plazaBuiltinModelCatalog["gemini-4-carbon"]},
	{Prefix: "gemini-4-argon", Entry: plazaBuiltinModelCatalog["gemini-4-argon"]},
	{Prefix: "gemini-3.1-flash-image", Entry: plazaBuiltinModelCatalog["gemini-3.1-flash-image"]},
	{Prefix: "gemini-3.1-pro", Entry: plazaBuiltinModelCatalog["gemini-3.1-pro"]},
	{Prefix: "gemini-3.8-flash", Entry: plazaBuiltinModelCatalog["gemini-3-flash"]},
	{Prefix: "gemini-3.7-flash", Entry: plazaBuiltinModelCatalog["gemini-3-flash"]},
	{Prefix: "gemini-3.6-flash", Entry: plazaBuiltinModelCatalog["gemini-3-flash"]},
	{Prefix: "gemini-3-pro-image", Entry: plazaBuiltinModelCatalog["gemini-3.1-flash-image"]},
	{Prefix: "gemini-3-pro", Entry: plazaBuiltinModelCatalog["gemini-3-pro"]},
	{Prefix: "gemini-3-flash", Entry: plazaBuiltinModelCatalog["gemini-3-flash"]},
	{Prefix: "gemini-2.5-flash-image", Entry: plazaBuiltinModelCatalog["gemini-2.5-flash-image"]},
	{Prefix: "gemini-2.5-flash-lite", Entry: plazaBuiltinModelCatalog["gemini-2.5-flash-lite"]},
	{Prefix: "gemini-2.5-flash", Entry: plazaBuiltinModelCatalog["gemini-2.5-flash"]},
	{Prefix: "gemini-2.5-pro", Entry: plazaBuiltinModelCatalog["gemini-2.5-pro"]},
	{Prefix: "grok-4.7", Entry: plazaBuiltinModelCatalog["grok-4.7"]},
	{Prefix: "grok-4.6", Entry: plazaBuiltinModelCatalog["grok-4.6"]},
	{Prefix: "grok-4.5", Entry: plazaBuiltinModelCatalog["grok-4.5"]},
	{Prefix: "grok-4-fast", Entry: plazaBuiltinModelCatalog["grok-4-fast"]},
	{Prefix: "grok-4", Entry: plazaBuiltinModelCatalog["grok-4"]},
	{Prefix: "grok-3-mini", Entry: plazaBuiltinModelCatalog["grok-3-mini"]},
	{Prefix: "grok-3", Entry: plazaBuiltinModelCatalog["grok-3"]},
	{Prefix: "deepseek-reasoner", Entry: plazaBuiltinModelCatalog["deepseek-reasoner"]},
	{Prefix: "deepseek-r1", Entry: plazaBuiltinModelCatalog["deepseek-r1"]},
	{Prefix: "deepseek-chat", Entry: plazaBuiltinModelCatalog["deepseek-chat"]},
	{Prefix: "deepseek-v3", Entry: plazaBuiltinModelCatalog["deepseek-v3"]},
	{Prefix: "kimi-k2.5", Entry: plazaBuiltinModelCatalog["kimi-k2.5"]},
	{Prefix: "kimi-k2", Entry: plazaBuiltinModelCatalog["kimi-k2"]},
	{Prefix: "glm-5", Entry: plazaBuiltinModelCatalog["glm-5"]},
	{Prefix: "glm-4.7", Entry: plazaBuiltinModelCatalog["glm-4.7"]},
	{Prefix: "glm-4.6", Entry: plazaBuiltinModelCatalog["glm-4.6"]},
	{Prefix: "glm-4.5", Entry: plazaBuiltinModelCatalog["glm-4.5"]},
	{Prefix: "minimax-m2.5", Entry: plazaBuiltinModelCatalog["minimax-m2.5"]},
	{Prefix: "minimax-m2", Entry: plazaBuiltinModelCatalog["minimax-m2"]},
	{Prefix: "qwen3-coder", Entry: plazaBuiltinModelCatalog["qwen3-coder"]},
	{Prefix: "qwen3-max", Entry: plazaBuiltinModelCatalog["qwen3-max"]},
}

// ResolvePlazaModelMetadata 融合三层来源（内置主流目录、PricingService 字段、前缀规则推断）
// 生成模型元数据，并按 Accept-Language 返回对应语言简介。
func ResolvePlazaModelMetadata(
	modelName string,
	platform string,
	channelPricing *ChannelModelPricing,
	pricingSvc *PricingService,
	acceptLanguage string,
) PlazaModelMetadata {
	trimmed := strings.TrimSpace(modelName)
	normalized := normalizePlazaMetadataModelKey(trimmed)
	isZH := preferredPlazaLanguageIsZH(acceptLanguage)

	meta := PlazaModelMetadata{
		DisplayName:  trimmed,
		Capabilities: []string{},
	}
	if meta.DisplayName == "" {
		meta.DisplayName = "Unknown"
	}

	capSet := make(map[string]struct{}, 8)
	addCap := func(capKey string) {
		if capKey == "" {
			return
		}
		if _, exists := capSet[capKey]; exists {
			return
		}
		capSet[capKey] = struct{}{}
		meta.Capabilities = append(meta.Capabilities, capKey)
	}

	// 第 1 层：内置主流模型元信息表（精确命中 → 家族前缀命中）
	if entry, exact, ok := lookupPlazaBuiltinCatalog(normalized); ok {
		if exact && entry.DisplayName != "" {
			meta.DisplayName = entry.DisplayName
		}
		if isZH {
			meta.Description = entry.DescriptionZH
		} else {
			meta.Description = entry.DescriptionEN
		}
		meta.Vendor = entry.Vendor
		if entry.ContextWindow > 0 {
			cw := entry.ContextWindow
			meta.ContextWindow = &cw
		}
		if entry.MaxOutputTokens > 0 {
			mo := entry.MaxOutputTokens
			meta.MaxOutputTokens = &mo
		}
		for _, c := range entry.Capabilities {
			addCap(c)
		}
	}

	// 第 2 层：补充读取 PricingService 已有目录字段
	var litellm *LiteLLMModelPricing
	if pricingSvc != nil && trimmed != "" {
		litellm = pricingSvc.GetModelPricing(trimmed)
	}
	if litellm != nil {
		if litellm.SupportsPromptCaching || litellm.CacheReadInputTokenCost > 0 {
			addCap("prompt_caching")
		}
		if litellm.LongContextInputTokenThreshold > 0 {
			addCap("long_context")
		}
		switch strings.ToLower(strings.TrimSpace(litellm.Mode)) {
		case "image_generation":
			addCap("image")
		case "embedding":
			addCap("embedding")
		case "audio_speech", "audio_transcription":
			addCap("audio")
		}
		if meta.Vendor == "" {
			meta.Vendor = mapLiteLLMProviderToPlazaVendor(litellm.LiteLLMProvider)
		}
	}

	// 第 3 层：未知/自定义别名模型按前缀与计费模式优雅推断
	if meta.Vendor == "" {
		meta.Vendor = inferPlazaModelVendor(normalized, platform)
	}
	if channelPricing != nil {
		switch channelPricing.BillingMode {
		case BillingModeImage:
			addCap("image")
		case BillingModeVideo:
			addCap("video")
		}
		if channelPricing.CacheReadPrice != nil && *channelPricing.CacheReadPrice > 0 {
			addCap("prompt_caching")
		}
	}
	inferPlazaFallbackCapabilities(normalized, platform, addCap)

	return meta
}

func normalizePlazaMetadataModelKey(modelName string) string {
	s := strings.ToLower(strings.TrimSpace(modelName))
	if s == "" {
		return ""
	}
	// 去除常见路径/区域前缀
	for _, prefix := range []string{
		"publishers/google/models/",
		"models/",
		"openai/",
		"anthropic/",
		"google/",
		"xai/",
		"x-ai/",
		"deepseek/",
		"global.anthropic.",
		"us.anthropic.",
		"eu.anthropic.",
		"apac.anthropic.",
		"jp.anthropic.",
		"anthropic.",
	} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimPrefix(s, prefix)
		}
	}
	s = plazaBedrockVersionRe.ReplaceAllString(s, "")
	s = plazaDateDashSuffixRe.ReplaceAllString(s, "")
	s = plazaDateCompactSuffixRe.ReplaceAllString(s, "")
	for _, suffix := range []string{"-latest", "-preview"} {
		s = strings.TrimSuffix(s, suffix)
	}
	return s
}

func lookupPlazaBuiltinCatalog(normalized string) (plazaModelCatalogEntry, bool, bool) {
	if normalized == "" {
		return plazaModelCatalogEntry{}, false, false
	}
	// 兼容 claude-sonnet-4.5 与 claude-sonnet-4-5 写法
	candidates := []string{normalized}
	if strings.HasPrefix(normalized, "claude-") && strings.Contains(normalized, ".") {
		candidates = append(candidates, strings.ReplaceAll(normalized, ".", "-"))
	}
	for _, cand := range candidates {
		if entry, ok := plazaBuiltinModelCatalog[cand]; ok {
			return entry, true, true
		}
	}
	for _, cand := range candidates {
		for _, rule := range plazaBuiltinPrefixRules {
			if cand == rule.Prefix || strings.HasPrefix(cand, rule.Prefix+"-") || strings.HasPrefix(cand, rule.Prefix+".") {
				return rule.Entry, false, true
			}
		}
	}
	return plazaModelCatalogEntry{}, false, false
}

func inferPlazaModelVendor(normalized, platform string) string {
	switch {
	case strings.HasPrefix(normalized, "gpt") ||
		strings.HasPrefix(normalized, "o1") ||
		strings.HasPrefix(normalized, "o3") ||
		strings.HasPrefix(normalized, "o4") ||
		strings.HasPrefix(normalized, "codex") ||
		strings.HasPrefix(normalized, "chatgpt") ||
		strings.HasPrefix(normalized, "dall-e") ||
		strings.HasPrefix(normalized, "whisper") ||
		strings.HasPrefix(normalized, "tts-") ||
		strings.HasPrefix(normalized, "text-embedding") ||
		strings.HasPrefix(normalized, "sora"):
		return "openai"
	case strings.HasPrefix(normalized, "claude"):
		return "anthropic"
	case strings.HasPrefix(normalized, "gemini") ||
		strings.HasPrefix(normalized, "gemma") ||
		strings.HasPrefix(normalized, "imagen") ||
		strings.HasPrefix(normalized, "veo"):
		return "google"
	case strings.HasPrefix(normalized, "grok"):
		return "xai"
	case strings.HasPrefix(normalized, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(normalized, "qwen") ||
		strings.HasPrefix(normalized, "qwq") ||
		strings.HasPrefix(normalized, "wanx"):
		return "qwen"
	case strings.HasPrefix(normalized, "glm") ||
		strings.HasPrefix(normalized, "chatglm") ||
		strings.HasPrefix(normalized, "cogview"):
		return "zhipu"
	case strings.HasPrefix(normalized, "kimi") ||
		strings.HasPrefix(normalized, "moonshot"):
		return "moonshot"
	case strings.HasPrefix(normalized, "minimax") ||
		strings.HasPrefix(normalized, "abab"):
		return "minimax"
	case strings.HasPrefix(normalized, "llama"):
		return "meta"
	case strings.HasPrefix(normalized, "mistral") ||
		strings.HasPrefix(normalized, "mixtral") ||
		strings.HasPrefix(normalized, "codestral") ||
		strings.HasPrefix(normalized, "pixtral"):
		return "mistral"
	case strings.HasPrefix(normalized, "doubao") ||
		strings.HasPrefix(normalized, "seedance") ||
		strings.HasPrefix(normalized, "seedream"):
		return "doubao"
	case strings.HasPrefix(normalized, "command") ||
		strings.HasPrefix(normalized, "cohere"):
		return "cohere"
	}
	return mapPlatformToPlazaVendor(platform)
}

func mapLiteLLMProviderToPlazaVendor(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai", "azure", "openai_text_completion":
		return "openai"
	case "anthropic", "bedrock_anthropic", "vertex_ai_anthropic":
		return "anthropic"
	case "gemini", "google", "vertex_ai", "vertex_ai-language-models", "vertex_ai-vision-models":
		return "google"
	case "xai":
		return "xai"
	case "deepseek":
		return "deepseek"
	case "qwen", "dashscope":
		return "qwen"
	case "zhipu", "zai":
		return "zhipu"
	case "moonshot":
		return "moonshot"
	case "minimax":
		return "minimax"
	case "mistral":
		return "mistral"
	case "meta_llama":
		return "meta"
	case "cohere", "cohere_chat":
		return "cohere"
	default:
		return ""
	}
}

func mapPlatformToPlazaVendor(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformOpenAI:
		return "openai"
	case PlatformAnthropic:
		return "anthropic"
	case PlatformGemini:
		return "google"
	case PlatformGrok:
		return "xai"
	case PlatformDeepseek:
		return "deepseek"
	case PlatformZhipu:
		return "zhipu"
	case PlatformKimi:
		return "moonshot"
	case PlatformMiniMax:
		return "minimax"
	default:
		return "other"
	}
}

func inferPlazaFallbackCapabilities(normalized, platform string, addCap func(string)) {
	if normalized == "" {
		return
	}
	if IsImageGenerationIntentForPlatform("", normalized, nil, platform) ||
		strings.Contains(normalized, "image") ||
		strings.Contains(normalized, "imagen") ||
		strings.Contains(normalized, "dall-e") ||
		strings.Contains(normalized, "seedream") ||
		strings.Contains(normalized, "flux") {
		addCap("image")
	}
	if strings.Contains(normalized, "video") ||
		strings.Contains(normalized, "sora") ||
		strings.Contains(normalized, "veo") ||
		strings.Contains(normalized, "seedance") {
		addCap("video")
	}
	if strings.Contains(normalized, "embed") {
		addCap("embedding")
	}
	if strings.Contains(normalized, "realtime") {
		addCap("realtime")
		addCap("audio")
	}
	if strings.Contains(normalized, "whisper") || strings.HasPrefix(normalized, "tts-") || strings.Contains(normalized, "audio") {
		addCap("audio")
	}
	if strings.Contains(normalized, "thinking") ||
		strings.Contains(normalized, "reasoner") ||
		strings.Contains(normalized, "-r1") ||
		strings.HasPrefix(normalized, "o1") ||
		strings.HasPrefix(normalized, "o3") ||
		strings.HasPrefix(normalized, "o4") {
		addCap("reasoning")
	}
	if strings.Contains(normalized, "codex") || strings.Contains(normalized, "coder") {
		addCap("code")
	}
}

// preferredPlazaLanguageIsZH 解析 Accept-Language 请求头，判断首选语言是否为中文。
func preferredPlazaLanguageIsZH(acceptLanguage string) bool {
	best, bestQ := "", 0.0
	for _, part := range strings.Split(acceptLanguage, ",") {
		tag, params, _ := strings.Cut(part, ";")
		tag = strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		if value, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil {
				continue
			}
			q = parsed
		}
		if q > bestQ {
			best, bestQ = tag, q
		}
	}
	if best == "" {
		return false
	}
	primary, _, _ := strings.Cut(strings.ToLower(best), "-")
	return primary == "zh"
}
