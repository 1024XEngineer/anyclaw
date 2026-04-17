package channels

import inputlayer "github.com/anyclaw/anyclaw/pkg/input"

type InboundHandler = inputlayer.InboundHandler
type StreamChunkHandler = inputlayer.StreamChunkHandler
type StreamAdapter = inputlayer.StreamAdapter
type Status = inputlayer.Status
type Adapter = inputlayer.Adapter
type BaseAdapter = inputlayer.BaseAdapter
type Manager = inputlayer.Manager

var NewBaseAdapter = inputlayer.NewBaseAdapter
var NewManager = inputlayer.NewManager
