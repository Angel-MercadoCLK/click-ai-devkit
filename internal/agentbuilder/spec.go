package agentbuilder

type Placement string

const (
	PlacementPersonal  Placement = "personal"
	PlacementShareable Placement = "shareable"
)

type AgentSpec struct {
	Engine      Engine
	Name        string
	Description string
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
