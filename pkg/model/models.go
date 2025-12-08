package model

import (
	"time"
)

// Operator represents the comparison operator.
type Operator string

const (
	OperatorEquals      Operator = "EQUALS"
	OperatorNotEquals   Operator = "NOT_EQUALS"
	OperatorGreaterThan Operator = "GREATER_THAN"
	OperatorLessThan    Operator = "LESS_THAN"
	OperatorContains    Operator = "CONTAINS"
	OperatorIn          Operator = "IN"
	OperatorNotIn       Operator = "NOT_IN"
	OperatorSplit       Operator = "SPLIT"
)

// Condition represents a rule condition.
type Condition struct {
	Variable string   `avro:"variable" json:"variable"`
	Operator Operator `avro:"operator" json:"operator"`
	Values   []string `avro:"values" json:"values"`
}

// Rule represents a rollout rule.
type Rule struct {
	Description   *string     `avro:"description" json:"description,omitempty"`
	Conditions    []Condition `avro:"conditions" json:"conditions"`
	TargetVersion string      `avro:"targetVersion" json:"targetVersion"`
}

// FigDefinition represents the metadata of a Fig.
type FigDefinition struct {
	Namespace     string    `avro:"namespace" json:"namespace"`
	Key           string    `avro:"key" json:"key"`
	FigID         string    `avro:"figId" json:"figId"`
	SchemaURI     string    `avro:"schemaUri" json:"schemaUri"`
	SchemaVersion string    `avro:"schemaVersion" json:"schemaVersion"`
	CreatedAt     time.Time `avro:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time `avro:"updatedAt" json:"updatedAt"`
}

// Fig represents a specific version of a configuration.
type Fig struct {
	FigID   string `avro:"figId" json:"figId"`
	Version string `avro:"version" json:"version"`
	Payload []byte `avro:"payload" json:"payload"`
}

// FigFamily represents a collection of Figs and rules for a key.
type FigFamily struct {
	Definition     FigDefinition `avro:"definition" json:"definition"`
	Figs           []Fig         `avro:"figs" json:"figs"`
	Rules          []Rule        `avro:"rules" json:"rules"`
	DefaultVersion *string       `avro:"defaultVersion" json:"defaultVersion,omitempty"`
}

// InitialFetchRequest represents the request for initial data.
type InitialFetchRequest struct {
	Namespace     string     `avro:"namespace" json:"namespace"`
	EnvironmentID string     `avro:"environmentId" json:"environmentId"`
	AsOfTimestamp *time.Time `avro:"asOfTimestamp" json:"asOfTimestamp,omitempty"`
}

// InitialFetchResponse represents the response for initial data.
type InitialFetchResponse struct {
	FigFamilies   []FigFamily `avro:"figFamilies" json:"figFamilies"`
	Cursor        string      `avro:"cursor" json:"cursor"`
	EnvironmentID string      `avro:"environmentId" json:"environmentId"`
}

// UpdateFetchRequest represents the request for updates.
type UpdateFetchRequest struct {
	Namespace     string `avro:"namespace" json:"namespace"`
	Cursor        string `avro:"cursor" json:"cursor"`
	EnvironmentID string `avro:"environmentId" json:"environmentId"`
}

// UpdateFetchResponse represents the response for updates.
type UpdateFetchResponse struct {
	FigFamilies []FigFamily `avro:"figFamilies" json:"figFamilies"`
	Cursor      string      `avro:"cursor" json:"cursor"`
}
