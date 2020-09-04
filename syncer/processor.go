package syncer

type RowData struct {
	Action string
	Schema string
	Table  string
	Data   map[string]interface{}
}
