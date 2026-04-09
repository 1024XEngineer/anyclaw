package cli

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"anyclaw/pairing"
	"github.com/spf13/cobra"
)

// Supported channels for pairing
var supportedChannels = []string{"telegram", "whatsapp", "signal", "imessage", "discord", "slack", "feishu"}

var pairingCmd = &cobra.Command{
	Use:   "pairing",
	Short: "Manage DM pairing for channel access control",
	Long: `Manage DM (direct message) pairing for controlling who can send messages to your bot.

Pairing is used when a channel's dmPolicy is set to "pairing". Unknown senders
will receive a pairing code and must be approved before their messages are processed.

Supported channels: telegram, whatsapp, signal, imessage, discord, slack, feishu`,
}

var pairingListCmd = &cobra.Command{
	Use:   "list <channel>",
	Short: "List pending pairing requests",
	Args:  cobra.ExactArgs(1),
	Run:   runPairingList,
}

var pairingApproveCmd = &cobra.Command{
	Use:   "approve <channel> <code>",
	Short: "Approve a pairing request",
	Args:  cobra.ExactArgs(2),
	Run:   runPairingApprove,
}

var pairingRejectCmd = &cobra.Command{
	Use:   "reject <channel> <code>",
	Short: "Reject a pairing request",
	Args:  cobra.ExactArgs(2),
	Run:   runPairingReject,
}

var pairingAllowlistCmd = &cobra.Command{
	Use:   "allowlist <channel>",
	Short: "Show allowlist for a channel",
	Args:  cobra.ExactArgs(1),
	Run:   runPairingAllowlist,
}

var pairingRemoveCmd = &cobra.Command{
	Use:   "remove <channel> <sender-id>",
	Short: "Remove a sender from the allowlist",
	Args:  cobra.ExactArgs(2),
	Run:   runPairingRemove,
}

var pairingAddCmd = &cobra.Command{
	Use:   "add <channel> <sender-id> <name>",
	Short: "Directly add a sender to the allowlist",
	Args:  cobra.ExactArgs(3),
	Run:   runPairingAdd,
}

// Account ID flag for multi-account channels
var pairingAccountID string

func init() {
	// Add subcommands
	pairingCmd.AddCommand(pairingListCmd)
	pairingCmd.AddCommand(pairingApproveCmd)
	pairingCmd.AddCommand(pairingRejectCmd)
	pairingCmd.AddCommand(pairingAllowlistCmd)
	pairingCmd.AddCommand(pairingRemoveCmd)
	pairingCmd.AddCommand(pairingAddCmd)

	// Add account-id flag for multi-account support
	pairingCmd.PersistentFlags().StringVar(&pairingAccountID, "account", "", "Account ID for multi-account channels (empty for default account)")
}

func runPairingList(cmd *cobra.Command, args []string) {
	channel := args[0]

	if !isSupportedChannel(channel) {
		fmt.Fprintf(os.Stderr, "Error: unsupported channel '%s'\n", channel)
		fmt.Fprintf(os.Stderr, "Supported channels: %v\n", supportedChannels)
		os.Exit(1)
	}

	store, err := pairing.NewPairingStore(pairing.Config{
		Channel:   channel,
		AccountID: pairingAccountID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create pairing store: %v\n", err)
		os.Exit(1)
	}

	pending := store.ListPending()

	if len(pending) == 0 {
		fmt.Printf("No pending pairing requests for channel '%s'\n", channel)
		return
	}

	// Use tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "CODE\tSENDER ID\tNAME\tEXPIRES\n")
	fmt.Fprintf(w, "----\t---------\t----\t-------\n")

	for _, req := range pending {
		expiresAt := fmtTime(req.ExpiresAt)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", req.Code, req.ID, req.Name, expiresAt)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d pending request(s)\n", len(pending))
}

func runPairingApprove(cmd *cobra.Command, args []string) {
	channel := args[0]
	code := args[1]

	if !isSupportedChannel(channel) {
		fmt.Fprintf(os.Stderr, "Error: unsupported channel '%s'\n", channel)
		fmt.Fprintf(os.Stderr, "Supported channels: %v\n", supportedChannels)
		os.Exit(1)
	}

	store, err := pairing.NewPairingStore(pairing.Config{
		Channel:   channel,
		AccountID: pairingAccountID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create pairing store: %v\n", err)
		os.Exit(1)
	}

	senderID, name, err := store.Approve(code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Approved pairing request\n")
	fmt.Printf("  Channel: %s\n", channel)
	fmt.Printf("  Sender ID: %s\n", senderID)
	fmt.Printf("  Name: %s\n", name)
	fmt.Printf("  Code: %s\n", code)

	if pairingAccountID != "" {
		fmt.Printf("  Account: %s\n", pairingAccountID)
	}
}

func runPairingReject(cmd *cobra.Command, args []string) {
	channel := args[0]
	code := args[1]

	if !isSupportedChannel(channel) {
		fmt.Fprintf(os.Stderr, "Error: unsupported channel '%s'\n", channel)
		fmt.Fprintf(os.Stderr, "Supported channels: %v\n", supportedChannels)
		os.Exit(1)
	}

	store, err := pairing.NewPairingStore(pairing.Config{
		Channel:   channel,
		AccountID: pairingAccountID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create pairing store: %v\n", err)
		os.Exit(1)
	}

	err = store.Reject(code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Rejected pairing request\n")
	fmt.Printf("  Channel: %s\n", channel)
	fmt.Printf("  Code: %s\n", code)
}

func runPairingAllowlist(cmd *cobra.Command, args []string) {
	channel := args[0]

	if !isSupportedChannel(channel) {
		fmt.Fprintf(os.Stderr, "Error: unsupported channel '%s'\n", channel)
		fmt.Fprintf(os.Stderr, "Supported channels: %v\n", supportedChannels)
		os.Exit(1)
	}

	store, err := pairing.NewPairingStore(pairing.Config{
		Channel:   channel,
		AccountID: pairingAccountID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create pairing store: %v\n", err)
		os.Exit(1)
	}

	allowlist := store.GetAllowlist()

	if len(allowlist) == 0 {
		fmt.Printf("Allowlist is empty for channel '%s'\n", channel)
		return
	}

	// Sort by sender ID for consistent output
	senderIDs := make([]string, 0, len(allowlist))
	for id := range allowlist {
		senderIDs = append(senderIDs, id)
	}
	sort.Strings(senderIDs)

	// Use tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "SENDER ID\tNAME\n")
	fmt.Fprintf(w, "---------\t----\n")

	for _, id := range senderIDs {
		name := allowlist[id]
		fmt.Fprintf(w, "%s\t%s\n", id, name)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d sender(s) in allowlist\n", len(allowlist))
}

func runPairingRemove(cmd *cobra.Command, args []string) {
	channel := args[0]
	senderID := args[1]

	if !isSupportedChannel(channel) {
		fmt.Fprintf(os.Stderr, "Error: unsupported channel '%s'\n", channel)
		fmt.Fprintf(os.Stderr, "Supported channels: %v\n", supportedChannels)
		os.Exit(1)
	}

	store, err := pairing.NewPairingStore(pairing.Config{
		Channel:   channel,
		AccountID: pairingAccountID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create pairing store: %v\n", err)
		os.Exit(1)
	}

	err = store.RemoveFromAllowlist(senderID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Removed sender from allowlist\n")
	fmt.Printf("  Channel: %s\n", channel)
	fmt.Printf("  Sender ID: %s\n", senderID)
}

func runPairingAdd(cmd *cobra.Command, args []string) {
	channel := args[0]
	senderID := args[1]
	name := args[2]

	if !isSupportedChannel(channel) {
		fmt.Fprintf(os.Stderr, "Error: unsupported channel '%s'\n", channel)
		fmt.Fprintf(os.Stderr, "Supported channels: %v\n", supportedChannels)
		os.Exit(1)
	}

	store, err := pairing.NewPairingStore(pairing.Config{
		Channel:   channel,
		AccountID: pairingAccountID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create pairing store: %v\n", err)
		os.Exit(1)
	}

	err = store.AddToAllowlist(senderID, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Added sender to allowlist\n")
	fmt.Printf("  Channel: %s\n", channel)
	fmt.Printf("  Sender ID: %s\n", senderID)
	fmt.Printf("  Name: %s\n", name)
}

func isSupportedChannel(channel string) bool {
	for _, c := range supportedChannels {
		if c == channel {
			return true
		}
	}
	return false
}

func fmtTime(unix int64) string {
	// Simple formatting - could use time.Unix().Format() for prettier output
	return fmt.Sprintf("%d", unix)
}
