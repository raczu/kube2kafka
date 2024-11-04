package config

import (
	"fmt"
	"github.com/raczu/kube2kafka/pkg/kube/watcher"
	"github.com/raczu/kube2kafka/pkg/processor"
	"gopkg.in/yaml.v3"
	"os"
	"time"
)

type Config struct {
	ClusterName     string               `yaml:"clusterName"`
	TargetNamespace string               `yaml:"namespace"`
	MaxEventAge     time.Duration        `yaml:"maxEventAge"`
	BufferSize      int                  `yaml:"bufferSize"`
	Kafka           *KafkaConfig         `yaml:"kafka"`
	Filters         []processor.Filter   `yaml:"filters"`
	Selectors       []processor.Selector `yaml:"selectors"`
}

func (c *Config) SetDefaults() {
	if c.MaxEventAge == 0 {
		c.MaxEventAge = watcher.DefaultMaxEventAge
	}

	if c.BufferSize == 0 {
		c.BufferSize = watcher.DefaultEventBufferCap
	}
}

func (c *Config) Validate() error {
	if c.ClusterName == "" {
		return fmt.Errorf("cluster name is required")
	}

	if c.BufferSize <= 0 {
		return fmt.Errorf("buffer size must be positive")
	}

	if c.Kafka == nil {
		return fmt.Errorf("kafka config is required")
	}

	if err := c.Kafka.Validate(); err != nil {
		return fmt.Errorf("kafka config has issues: %w", err)
	}

	for i, filter := range c.Filters {
		err := filter.Validate()
		if err != nil {
			return fmt.Errorf("filter at index %d has issues: %w", i, err)
		}
	}

	for i, selector := range c.Selectors {
		err := selector.Validate()
		if err != nil {
			return fmt.Errorf("selector at index %d has issues: %w", i, err)
		}
	}
	return nil
}

func Read(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
