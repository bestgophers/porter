package utils

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

type Resp struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewResp creates a Resp
func NewResp() *Resp {
	return &Resp{
		Message: "success",
	}
}

//SetData set data into Resp
func (r *Resp) SetData(data interface{}) *Resp {
	r.Data = data
	return r
}

//SetError set error into Resp
func (r *Resp) SetError(msg string) *Resp {
	r.Message = msg
	return r
}

//SendRequest send PUT request to leader
func SendRequest(method string, leaderURL string, data []byte) (*Resp, error) {
	client := &http.Client{}
	req, err := http.NewRequest(method, leaderURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json;charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	r := new(Resp)
	err = json.Unmarshal(respBody, r)
	if err != nil {
		return nil, err
	}

	return r, nil
}
