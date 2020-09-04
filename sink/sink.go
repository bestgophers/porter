package sink

import (
	"fmt"
	"github.com/siddontang/go-mysql/client"
)

type Sink struct {
	conn *client.Conn
}

func NewSink() *Sink {
	return &Sink{}
}

func (s *Sink) Start() (*client.Conn, error) {
	connect, err := client.Connect("127.0.0.1:3306", "root", "yaoyichen52", "test")
	if err != nil {
		return nil, err
	}
	err = connect.Ping()
	if err != nil {
		return nil, err
	}
	return connect, nil
}

func (s *Sink) Insert(sql string) error {
	execute, err := s.conn.Execute(sql)
	if err != nil {
		return err
	}
	fmt.Printf("insert sql id: %s", execute.InsertId)

	return nil
}

func (s *Sink) Stop() error {
	err := s.conn.Close()

	if err != nil {
		return err
	}
	return nil
}
