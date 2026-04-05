// Package channel re-exports types from the channels package for backward compatibility.
// The channels/ directory contains package channel (singular).
package channel

// Re-export all types from the channels package (which declares "package channel")
import (
	ch "github.com/anyclaw/anyclaw/pkg/channels"
)

type Adapter = ch.Adapter
type InboundHandler = ch.InboundHandler
type StreamChunkHandler = ch.StreamChunkHandler
type StreamAdapter = ch.StreamAdapter
type Status = ch.Status
type BaseAdapter = ch.BaseAdapter
type Manager = ch.Manager
type RouteRequest = ch.RouteRequest
type RouteDecision = ch.RouteDecision
type Router = ch.Router
type ChannelCommands = ch.ChannelCommands
type CommandHandler = ch.CommandHandler
type MentionGate = ch.MentionGate
type GroupSecurity = ch.GroupSecurity
type ChannelPairing = ch.ChannelPairing
type PairingInfo = ch.PairingInfo
type PresenceManager = ch.PresenceManager
type PresenceInfo = ch.PresenceInfo
type ContactDirectory = ch.ContactDirectory
type ContactInfo = ch.ContactInfo
type ChannelPolicy = ch.ChannelPolicy
type DMPolicy = ch.DMPolicy
type GroupPolicy = ch.GroupPolicy
type SecurityAuditResult = ch.SecurityAuditResult
type SecurityAuditIssue = ch.SecurityAuditIssue

var NewBaseAdapter = ch.NewBaseAdapter
var NewManager = ch.NewManager
var NewRouter = ch.NewRouter
var NewChannelCommands = ch.NewChannelCommands
var NewMentionGate = ch.NewMentionGate
var NewGroupSecurity = ch.NewGroupSecurity
var NewChannelPairing = ch.NewChannelPairing
var NewPresenceManager = ch.NewPresenceManager
var NewContactDirectory = ch.NewContactDirectory

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
var DefaultChannelPolicy = ch.DefaultChannelPolicy
var ChannelPolicyFromConfig = ch.ChannelPolicyFromConfig
var AuditChannelPolicy = ch.AuditChannelPolicy
