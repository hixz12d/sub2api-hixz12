package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/setting"
)

// nil expected means the row must not exist. A duplicate insert or changed
// value loses the CAS; there is no unconditional upsert fallback.
func (r *settingRepository) CompareAndSwapSetting(ctx context.Context, key string, expected *string, value string) (bool, error) {
	if expected == nil {
		err := r.client.Setting.Create().SetKey(key).SetValue(value).SetUpdatedAt(time.Now()).Exec(ctx)
		if ent.IsConstraintError(err) {
			return false, nil
		}
		return err == nil, err
	}
	count, err := r.client.Setting.Update().Where(setting.KeyEQ(key), setting.ValueEQ(*expected)).SetValue(value).SetUpdatedAt(time.Now()).Save(ctx)
	return count == 1, err
}
