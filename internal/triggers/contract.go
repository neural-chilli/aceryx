package triggers

import (
	"fmt"
	"strings"

	"github.com/neural-chilli/aceryx/internal/plugins"
)

type TriggerContract struct {
	Delivery    DeliveryMode     `yaml:"delivery" json:"delivery"`
	State       StateMode        `yaml:"state" json:"state"`
	Concurrency ConcurrencyMode  `yaml:"concurrency" json:"concurrency"`
	Ordering    OrderingMode     `yaml:"ordering" json:"ordering"`
	Checkpoint  CheckpointConfig `yaml:"checkpoint" json:"checkpoint"`
}

type DeliveryMode string

const (
	DeliveryAtLeastOnce DeliveryMode = "at_least_once"
	DeliveryExactlyOnce DeliveryMode = "exactly_once"
	DeliveryBestEffort  DeliveryMode = "best_effort"
)

type StateMode string

const (
	StateHostManaged   StateMode = "host_managed"
	StatePluginManaged StateMode = "plugin_managed"
)

type ConcurrencyMode string

const (
	ConcurrencySingle   ConcurrencyMode = "single"
	ConcurrencyParallel ConcurrencyMode = "parallel"
)

type OrderingMode string

const (
	OrderingOrdered   OrderingMode = "ordered"
	OrderingUnordered OrderingMode = "unordered"
)

type CheckpointConfig struct {
	Strategy   CheckpointStrategy `yaml:"strategy" json:"strategy"`
	IntervalMS int                `yaml:"interval_ms" json:"interval_ms"`
}

type CheckpointStrategy string

const (
	CheckpointPerMessage CheckpointStrategy = "per_message"
	CheckpointPeriodic   CheckpointStrategy = "periodic"
	CheckpointOnShutdown CheckpointStrategy = "on_shutdown"
)

func DefaultTriggerContract() TriggerContract {
	return TriggerContract{
		Delivery:    DeliveryAtLeastOnce,
		State:       StateHostManaged,
		Concurrency: ConcurrencySingle,
		Ordering:    OrderingOrdered,
		Checkpoint: CheckpointConfig{
			Strategy:   CheckpointPerMessage,
			IntervalMS: 5000,
		},
	}
}

func ParseTriggerContract(in *plugins.TriggerContract) (TriggerContract, error) {
	out := DefaultTriggerContract()
	if in == nil {
		return out, nil
	}
	if v := DeliveryMode(strings.TrimSpace(in.Delivery)); v != "" {
		out.Delivery = v
	}
	if v := StateMode(strings.TrimSpace(in.State)); v != "" {
		out.State = v
	}
	if v := ConcurrencyMode(strings.TrimSpace(in.Concurrency)); v != "" {
		out.Concurrency = v
	}
	if v := OrderingMode(strings.TrimSpace(in.Ordering)); v != "" {
		out.Ordering = v
	}
	if in.Checkpoint != nil {
		if v := CheckpointStrategy(strings.TrimSpace(in.Checkpoint.Strategy)); v != "" {
			out.Checkpoint.Strategy = v
		}
		if in.Checkpoint.IntervalMS > 0 {
			out.Checkpoint.IntervalMS = in.Checkpoint.IntervalMS
		}
	}
	if err := out.Validate(); err != nil {
		return TriggerContract{}, err
	}
	return out, nil
}

func (c TriggerContract) Validate() error {
	switch c.Delivery {
	case DeliveryAtLeastOnce, DeliveryExactlyOnce, DeliveryBestEffort:
	default:
		return fmt.Errorf("invalid delivery mode: %s", c.Delivery)
	}
	switch c.State {
	case StateHostManaged, StatePluginManaged:
	default:
		return fmt.Errorf("invalid state mode: %s", c.State)
	}
	switch c.Concurrency {
	case ConcurrencySingle, ConcurrencyParallel:
	default:
		return fmt.Errorf("invalid concurrency mode: %s", c.Concurrency)
	}
	switch c.Ordering {
	case OrderingOrdered, OrderingUnordered:
	default:
		return fmt.Errorf("invalid ordering mode: %s", c.Ordering)
	}
	switch c.Checkpoint.Strategy {
	case CheckpointPerMessage, CheckpointPeriodic, CheckpointOnShutdown:
	default:
		return fmt.Errorf("invalid checkpoint strategy: %s", c.Checkpoint.Strategy)
	}
	if c.Checkpoint.Strategy == CheckpointPeriodic && c.Checkpoint.IntervalMS <= 0 {
		return fmt.Errorf("checkpoint interval_ms must be > 0 for periodic strategy")
	}
	return nil
}
