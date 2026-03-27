package nrd

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ICANN struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"icann"`
	
	NRDList struct {
		Phase1 SourceConfig `yaml:"phase1"`
		Phase2 SourceConfig `yaml:"phase2"`
		Phase3 SourceConfig `yaml:"phase3"`
		Phase4 SourceConfig `yaml:"phase4"`
	} `yaml:"nrdlist"`
	
	Database DatabaseConfig `yaml:"database"`
}

type SourceConfig struct {
	URL string `yaml:"url"`
	Ext string `yaml:"ext"`
}

// LoadConfig 從 YAML 檔案載入配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
