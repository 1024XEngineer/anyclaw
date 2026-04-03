// Package channel re-exports types from the channels package for backward compatibility.
// The channels/ directory contains package channel (singular).
package channel

// Re-export all types from the channels package (which declares "package channel")
import (
	ch "github.com/anyclaw/anyclaw/pkg/channels"
)

type Adapter = ch.Adapter
type InboundHandler = ch.InboundHandler
type Status = ch.Status
type BaseAdapter = ch.BaseAdapter
type Manager = ch.Manager
type RouteRequest = ch.RouteRequest
type RouteDecision = ch.RouteDecision
type Router = ch.Router

var NewBaseAdapter = ch.NewBaseAdapter
var NewManager = ch.NewManager
var NewRouter = ch.NewRouter

var NewTelegramAdapter = ch.NewTelegramAdapter
var NewSlackAdapter = ch.NewSlackAdapter
var NewDiscordAdapter = ch.NewDiscordAdapter
var NewWhatsAppAdapter = ch.NewWhatsAppAdapter
var NewSignalAdapter = ch.NewSignalAdapter

type TelegramAdapter = ch.TelegramAdapter
type SlackAdapter = ch.SlackAdapter
type DiscordAdapter = ch.DiscordAdapter
type WhatsAppAdapter = ch.WhatsAppAdapter
type SignalAdapter = ch.SignalAdapter

var ReadBody = ch.ReadBody
var AnalyzeRouting = ch.AnalyzeRouting
