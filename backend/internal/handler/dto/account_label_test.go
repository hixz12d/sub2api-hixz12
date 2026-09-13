package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserAccountLabel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, want string }{
		{"Team（hixz2612） 子号", "hix**12"},
		{"Team (hixz2613) 子号", "hix**13"},
		{"Team（futureowner987） 子号\nprivate@example.com", "fut**87"},
		{" team（ 新增测试归属名字 ） 子号", "新增测**名字"},
		{"Team（a） 子号", "a**"},
		{"Team（abcde）", "a**"},
		{"Pro 263", "263"},
		{"Pro BilaHanzely69", "BilaHa"},
		{"Pro newxiaozhu.2", "newxia"},
		{"Pro .19", ".19"},
		{"Pro（future1234）", "future"},
		{"Pro  新增测试归属名字", "新增测试归属"},
		{"Pro 263\nprivate@example.com", "263"},
		{"Team（owner123@example.com）", "own**23"},
		{"Pro future123@example.com", "future"},
		{"", ""},
		{"Team（） 子号", ""},
		{"Team（@example.com）", ""},
		{"Team（secret name）", ""},
		{"Pro", ""},
		{"Profile private@example.com", ""},
		{"private@example.com", ""},
		{"unrecognized account name", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, userAccountLabel(&service.Account{Name: tc.name}))
		})
	}
	require.Empty(t, userAccountLabel(nil))
}

func TestUsageLogAccountLabelPrivacy(t *testing.T) {
	t.Parallel()
	log := &service.UsageLog{AccountID: 42, Account: &service.Account{
		ID: 42, Name: "Team（futureowner987） 子号\nprivate@example.com",
		Credentials: map[string]any{"access_token": "secret-token"},
	}}
	user := UsageLogFromService(log)
	require.Equal(t, "fut**87", user.AccountLabel)
	payload, err := json.Marshal(user)
	require.NoError(t, err)
	for _, secret := range []string{"futureowner987", "private@example.com", "secret-token", `"account":`, "credentials"} {
		require.NotContains(t, string(payload), secret)
	}
	admin := UsageLogFromServiceAdmin(log)
	require.Equal(t, log.Account.Name, admin.Account.Name)
	require.Empty(t, admin.AccountLabel)
	log.Account = nil
	payload, err = json.Marshal(UsageLogFromService(log))
	require.NoError(t, err)
	require.NotContains(t, string(payload), "account_label")
}
