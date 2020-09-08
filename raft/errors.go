package raft

import "errors"

var (
	ErrIdRemoved     = errors.New("membership: id is removed")
	ErrIdExists      = errors.New("membership: id exists")
	ErrIdNotFound    = errors.New("membership: id not fount")
	ErrPeerUrlExists = errors.New("membership: peerUrl exists")
)
