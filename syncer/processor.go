package syncer

type Processor struct {
	Action string
	Schema string
	Table  string
	Data   map[string]interface{}
}
