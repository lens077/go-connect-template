package config

import (
	"bytes"
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/spf13/viper"
)

func parseYAMLToMap(data []byte) (map[string]interface{}, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewBuffer(data)); err != nil {
		return nil, err
	}
	return v.AllSettings(), nil
}

func GetConfigFromConsul(client *api.Client, path string) (map[string]interface{}, error) {
	kv := client.KV()
	pair, _, err := kv.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("consul kv get failed: %w", err)
	}

	if pair == nil || len(pair.Value) == 0 {
		return nil, fmt.Errorf("config path is empty: %s", path)
	}

	return parseYAMLToMap(pair.Value)
}
