package service

import (
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const openAIForceStoreFalseKey = "openai_force_store_false"

func openAIForceStoreFalse(account *Account) bool {
	if account == nil || !account.IsOpenAIApiKey() {
		return false
	}
	enabled, _ := account.Extra[openAIForceStoreFalseKey].(bool)
	return enabled
}

// Opt-in for stateless Responses upstreams. Compact has a different wire schema.
func applyOpenAIStorePolicy(body []byte, account *Account, compact bool) ([]byte, error) {
	if compact || !openAIForceStoreFalse(account) || len(body) == 0 {
		return body, nil
	}
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, fmt.Errorf("apply Responses store policy: expected a JSON object")
	}
	if gjson.GetBytes(body, "store").Type == gjson.False {
		return body, nil
	}
	return sjson.SetBytes(body, "store", false)
}
