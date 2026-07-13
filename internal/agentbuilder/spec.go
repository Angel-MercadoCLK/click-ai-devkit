package agentbuilder

import "github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"

type SDDMode string

const (
	SDDStandalone   SDDMode = "standalone"
	SDDPhaseSupport SDDMode = "phase-support"
)

func SDDModes() []SDDMode {
	return []SDDMode{SDDStandalone, SDDPhaseSupport}
}

type Placement string

const (
	PlacementPersonal  Placement = "personal"
	PlacementShareable Placement = "shareable"
)

type AgentSpec struct {
	Engine      Engine
	Name        string
	Description string
	SDDMode     SDDMode
	Phase       modelconfig.Phase
	Tools       string
	Model       string
	Purpose     string
	Tasks       string
	Triggers    string
	Rules       string
	Tone        string
	Domain      string
	GoodOutput  string
	Placement   Placement
}
