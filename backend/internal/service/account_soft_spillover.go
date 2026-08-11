package service

import "context"

const defaultSoftSpilloverThresholdPercent = 80

func normalizeSoftSpilloverThresholdPercent(percent int) int {
	if percent <= 0 {
		return defaultSoftSpilloverThresholdPercent
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func softSpilloverConcurrencyThreshold(maxConcurrency, percent int) int {
	if maxConcurrency <= 0 {
		return 0
	}
	percent = normalizeSoftSpilloverThresholdPercent(percent)
	threshold := (maxConcurrency*percent + 99) / 100
	if threshold < 1 {
		threshold = 1
	}
	if threshold > maxConcurrency {
		threshold = maxConcurrency
	}
	return threshold
}

func accountLoadReachedSoftSpillover(load *AccountLoadInfo, maxConcurrency, percent int) bool {
	if load == nil || maxConcurrency <= 0 {
		return false
	}
	threshold := softSpilloverConcurrencyThreshold(maxConcurrency, percent)
	return threshold > 0 && load.CurrentConcurrency+load.WaitingCount >= threshold
}

func fetchAccountSoftSpilloverState(
	ctx context.Context,
	concurrencyService *ConcurrencyService,
	accountID int64,
	maxConcurrency int,
	percent int,
) (bool, *AccountLoadInfo) {
	if concurrencyService == nil || accountID <= 0 || maxConcurrency <= 0 {
		return false, nil
	}
	loadMap, err := concurrencyService.GetAccountsLoadBatch(ctx, []AccountWithConcurrency{{
		ID:             accountID,
		MaxConcurrency: maxConcurrency,
	}})
	if err != nil {
		return false, nil
	}
	load := loadMap[accountID]
	return accountLoadReachedSoftSpillover(load, maxConcurrency, percent), load
}

func fetchAccountSoftSpilloverStateFresh(
	ctx context.Context,
	concurrencyService *ConcurrencyService,
	accountID int64,
	maxConcurrency int,
	percent int,
) (bool, *AccountLoadInfo, error) {
	if concurrencyService == nil || accountID <= 0 || maxConcurrency <= 0 {
		return false, nil, nil
	}
	loadMap, err := concurrencyService.GetAccountsLoadBatchFresh(ctx, []AccountWithConcurrency{{
		ID:             accountID,
		MaxConcurrency: maxConcurrency,
	}})
	if err != nil {
		return false, nil, err
	}
	load := loadMap[accountID]
	return accountLoadReachedSoftSpillover(load, maxConcurrency, percent), load, nil
}

func fetchAccountBelowSoftSpilloverThresholdFresh(
	ctx context.Context,
	concurrencyService *ConcurrencyService,
	accountID int64,
	maxConcurrency int,
	percent int,
) (bool, error) {
	reached, _, err := fetchAccountSoftSpilloverStateFresh(
		ctx, concurrencyService, accountID, maxConcurrency, percent,
	)
	return !reached, err
}
