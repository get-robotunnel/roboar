// Package model defines the registry entity types exactly as serialized over the
// API (spec §1). JSONB columns are carried as json.RawMessage so the server stays
// schema-agnostic about their contents.
package model

import (
	"encoding/json"
	"time"
)

type Owner struct {
	OwnerID     string    `json:"owner_id"`
	PublicKey   string    `json:"public_key"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Platform struct {
	PlatformID   string     `json:"platform_id"`
	OwnerID      string     `json:"owner_id"`
	PlatformType string     `json:"platform_type"`
	DisplayName  string     `json:"display_name"`
	Description  string     `json:"description,omitempty"`
	PublicKey    string     `json:"public_key,omitempty"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	Online       bool       `json:"online"`
	Tags         []string   `json:"tags"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Connection struct {
	TunnelEndpoint string `json:"tunnel_endpoint,omitempty"`
	MCPEndpoint    string `json:"mcp_endpoint,omitempty"`
	RestEndpoint   string `json:"rest_endpoint,omitempty"`
}

type Agent struct {
	AgentID      string          `json:"agent_id"`
	PlatformID   string          `json:"platform_id"`
	OwnerID      string          `json:"owner_id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	AgentType    string          `json:"agent_type"`
	Version      string          `json:"version"`
	Status       string          `json:"status"`
	Visibility   string          `json:"visibility"`
	Connection   Connection      `json:"connection"`
	Capabilities []Capability    `json:"capabilities"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type Capability struct {
	CapabilityID     string          `json:"capability_id"`
	AgentID          string          `json:"agent_id"`
	Name             string          `json:"name"`
	DisplayName      string          `json:"display_name"`
	Description      string          `json:"description"`
	InterfaceType    string          `json:"interface_type"`
	Permission       string          `json:"permission"`
	Pricing          json.RawMessage `json:"pricing,omitempty"`
	AuthorizedAgents []string        `json:"authorized_agents,omitempty"`
	InputSchema      json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema     json.RawMessage `json:"output_schema,omitempty"`
	ROS2             json.RawMessage `json:"ros2,omitempty"`
	Enabled          bool            `json:"enabled"`
	CreatedAt        time.Time       `json:"created_at"`
}
