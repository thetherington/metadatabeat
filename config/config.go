// Config is put into a different package to prevent cyclic imports in case
// it is needed in several locations

package config

type Config struct {
	Port       int      `config:"port"`
	Addresses  []string `config:"addresses"`
	Interfaces []string `config:"interfaces"`
}

var DefaultConfig = Config{
	Port:       5010,
	Addresses:  []string{"239.131.10.225"},
	Interfaces: []string{"enp0s31f6"},
}
