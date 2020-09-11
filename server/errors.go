package server

import "errors"

var (
	ErrStatusStop  = errors.New("Syncer status is stop, please invoke [ StopSyncer ] first")
	ErrInvalidPara = errors.New("Invalid paramater")
)
