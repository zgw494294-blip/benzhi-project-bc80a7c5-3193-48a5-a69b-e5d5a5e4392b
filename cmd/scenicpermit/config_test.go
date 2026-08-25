package main

import "testing"

func TestConfigAddressPrecedence(t *testing.T) {
	env := func(name string) (string, bool) { return "19123", true }
	cfg, err := parseConfig(nil, env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != "127.0.0.1:19123" {
		t.Fatalf("地址 = %s", cfg.Address)
	}
	cfg, err = parseConfig([]string{"-addr=127.0.0.1:19234"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != "127.0.0.1:19234" {
		t.Fatalf("显式地址未优先: %s", cfg.Address)
	}
}

func TestConfigRejectsWildcard(t *testing.T) {
	_, err := parseConfig([]string{"-addr=0.0.0.0:19081"}, func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatal("全网监听地址应被拒绝")
	}
}
