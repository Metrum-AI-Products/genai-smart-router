// Package smartrouterctl implements customer-local file-owned operations and
// Kubernetes architecture blueprint rendering for GenAI Smart Router.
// It never calls AWS, EKS, RDS, Fleet, or the Kubernetes API.
package smartrouterctl

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const IntentSchema = "metrum.ai/smartrouter-stack-intent/v1"

// StackIntent is a non-Kubernetes input that describes a deployable stack.
type StackIntent struct {
	Schema            string               `yaml:"schema" json:"schema"`
	Profile           string               `yaml:"profile" json:"profile"`
	Namespace         string               `yaml:"namespace" json:"namespace"`
	RouterImage       string               `yaml:"router_image" json:"router_image"`
	Edge              string               `yaml:"edge" json:"edge"`
	UsageDriver       string               `yaml:"usage_driver" json:"usage_driver"`
	GPUOperator       GPUOperatorIntent    `yaml:"gpu_operator" json:"gpu_operator"`
	KVCache           KVCacheIntent        `yaml:"kv_cache" json:"kv_cache"`
	Packaging         PackagingIntent      `yaml:"packaging" json:"packaging"`
	ServingModels     []ServingModelIntent `yaml:"serving_models" json:"serving_models"`
	DefaultModelGroup string               `yaml:"default_model_group" json:"default_model_group"`
	CallerAllow       []string             `yaml:"caller_allow" json:"caller_allow"`
}

// PackagingIntent controls generated Helm chart and optional CRD operator scaffolds.
type PackagingIntent struct {
	// Helm defaults to true so Level-1 chart generation is part of blueprint render.
	Helm *bool `yaml:"helm" json:"helm"`
	// Operator defaults to false; set true when Level-2 CRD scaffold is required.
	Operator bool `yaml:"operator" json:"operator"`
}

func (p PackagingIntent) HelmEnabled() bool {
	if p.Helm == nil {
		return true
	}
	return *p.Helm
}

type GPUOperatorIntent struct {
	Enabled          bool   `yaml:"enabled" json:"enabled"`
	Version          string `yaml:"version" json:"version"`
	DriverOwnedByAMI bool   `yaml:"driver_owned_by_ami" json:"driver_owned_by_ami"`
}

type KVCacheIntent struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type ServingModelIntent struct {
	Name          string `yaml:"name" json:"name"`
	ServedModelID string `yaml:"served_model_id" json:"served_model_id"`
	HuggingFaceID string `yaml:"huggingface_id" json:"huggingface_id"`
	Image         string `yaml:"image" json:"image"`
	GPUCount      int    `yaml:"gpu_count" json:"gpu_count"`
	Port          int    `yaml:"port" json:"port"`
	ModelGroup    string `yaml:"model_group" json:"model_group"`
	ServiceDNS    string `yaml:"service_dns" json:"service_dns"`
	APIKeyEnv     string `yaml:"api_key_env" json:"api_key_env"`
}

// LoadIntent reads and validates a stack intent YAML file.
func LoadIntent(path string) (*StackIntent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var intent StackIntent
	if err := yaml.Unmarshal(raw, &intent); err != nil {
		return nil, fmt.Errorf("parse intent: %w", err)
	}
	if err := intent.Validate(); err != nil {
		return nil, err
	}
	return &intent, nil
}

func (i *StackIntent) Validate() error {
	if i == nil {
		return fmt.Errorf("intent is required")
	}
	if strings.TrimSpace(i.Schema) == "" {
		i.Schema = IntentSchema
	}
	if i.Schema != IntentSchema {
		return fmt.Errorf("unsupported intent schema %q", i.Schema)
	}
	switch strings.TrimSpace(i.Profile) {
	case "minimal", "nvidia-local-serving":
	default:
		return fmt.Errorf("profile must be minimal or nvidia-local-serving")
	}
	if strings.TrimSpace(i.Namespace) == "" {
		i.Namespace = "smart-llmrouter"
	}
	if strings.TrimSpace(i.Edge) == "" {
		i.Edge = "ingress"
	}
	switch i.Edge {
	case "caddy", "ingress", "none":
	default:
		return fmt.Errorf("edge must be caddy, ingress, or none")
	}
	if strings.TrimSpace(i.UsageDriver) == "" {
		i.UsageDriver = "sqlite"
	}
	if i.UsageDriver != "sqlite" && i.UsageDriver != "postgres" {
		return fmt.Errorf("usage_driver must be sqlite or postgres")
	}
	if i.Profile == "nvidia-local-serving" {
		if !i.GPUOperator.Enabled {
			return fmt.Errorf("nvidia-local-serving requires gpu_operator.enabled")
		}
		if len(i.ServingModels) < 2 {
			return fmt.Errorf("nvidia-local-serving requires at least two serving_models")
		}
	}
	if i.GPUOperator.Enabled && strings.TrimSpace(i.GPUOperator.Version) == "" {
		i.GPUOperator.Version = "v26.7.0"
	}
	seenGroups := map[string]struct{}{}
	for idx, model := range i.ServingModels {
		if strings.TrimSpace(model.Name) == "" {
			return fmt.Errorf("serving_models[%d].name is required", idx)
		}
		if strings.TrimSpace(model.ServedModelID) == "" {
			return fmt.Errorf("serving_models[%d].served_model_id is required", idx)
		}
		if model.GPUCount < 1 {
			i.ServingModels[idx].GPUCount = 1
		}
		if model.Port < 1 {
			i.ServingModels[idx].Port = 8000
		}
		if strings.TrimSpace(model.ModelGroup) == "" {
			i.ServingModels[idx].ModelGroup = model.Name
		}
		if strings.TrimSpace(model.ServiceDNS) == "" {
			i.ServingModels[idx].ServiceDNS = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/v1", model.Name, i.Namespace, i.ServingModels[idx].Port)
		}
		if strings.TrimSpace(model.APIKeyEnv) == "" {
			i.ServingModels[idx].APIKeyEnv = "LOCAL_VLLM_API_KEY"
		}
		if strings.TrimSpace(model.Image) == "" {
			return fmt.Errorf("serving_models[%d].image is required", idx)
		}
		seenGroups[i.ServingModels[idx].ModelGroup] = struct{}{}
	}
	if strings.TrimSpace(i.DefaultModelGroup) == "" && len(i.ServingModels) > 0 {
		i.DefaultModelGroup = i.ServingModels[0].ModelGroup
	}
	if len(i.CallerAllow) == 0 {
		groups := make([]string, 0, len(seenGroups))
		for group := range seenGroups {
			groups = append(groups, group)
		}
		sort.Strings(groups)
		i.CallerAllow = groups
	}
	return nil
}
