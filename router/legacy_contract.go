package router

import (
	"encoding/json"
	"sort"
	"strings"

	"gochen/errorx"
	"gochen/httpx"
)

func rejectLegacyQueryParams(ctx httpx.IContext, keys ...string) error {
	if ctx == nil {
		return nil
	}
	params := ctx.GetQueryParams()
	if len(params) == 0 {
		return nil
	}

	var found []string
	for _, key := range keys {
		if values, ok := params[key]; ok && len(values) > 0 {
			found = append(found, key)
		}
	}
	if len(found) == 0 {
		return nil
	}
	sort.Strings(found)
	return errorx.New(errorx.InvalidInput, "legacy query params are no longer supported; use filter/page/size DSL").
		WithContext("legacy_params", strings.Join(found, ","))
}

func rejectLegacyJSONFields(ctx httpx.IContext, keys ...string) error {
	if ctx == nil {
		return nil
	}
	body, err := ctx.GetBody()
	if err != nil || len(body) == 0 {
		return err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil
	}

	var found []string
	for _, key := range keys {
		if _, ok := fields[key]; ok {
			found = append(found, key)
		}
	}
	if len(found) == 0 {
		return nil
	}
	sort.Strings(found)
	return errorx.New(errorx.InvalidInput, "legacy json fields are no longer supported; use the typed contract").
		WithContext("legacy_fields", strings.Join(found, ","))
}
