package main

import (
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/agentstore"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

func runStoreCommand(args []string) error {
	if len(args) == 0 {
		printStoreUsage()
		return nil
	}

	switch args[0] {
	case "list":
		return runStoreList(args[1:])
	case "search":
		return runStoreSearch(args[1:])
	case "info":
		return runStoreInfo(args[1:])
	case "install":
		return runStoreInstall(args[1:])
	case "uninstall":
		return runStoreUninstall(args[1:])
	default:
		printStoreUsage()
		return fmt.Errorf("unknown store command: %s", args[0])
	}
}

func printStoreUsage() {
	fmt.Print(`AnyClaw store commands:

Usage:
  anyclaw store list [category]
  anyclaw store search <keyword>
  anyclaw store info <id>
  anyclaw store install <id>
  anyclaw store uninstall <id>
`)
}

func runStoreList(args []string) error {
	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		return err
	}

	filter := agentstore.StoreFilter{}
	if len(args) > 0 {
		filter.Category = args[0]
	}

	packages := sm.List(filter)
	if len(packages) == 0 {
		fmt.Println("No packages found.")
		return nil
	}

	fmt.Println(ui.Bold.Sprint(fmt.Sprintf("Packages (%d):", len(packages))))
	fmt.Println()
	for _, pkg := range packages {
		icon := pkg.Icon
		if icon == "" {
			icon = "-"
		}
		installed := ""
		if sm.IsInstalled(pkg.ID) {
			installed = ui.Green.Sprint(" [installed]")
		}
		fmt.Println("  " + icon + " " + ui.Bold.Sprint(pkg.DisplayName) + installed)
		fmt.Println("     " + ui.Dim.Sprint(pkg.Description))
		fmt.Println(fmt.Sprintf("     category: %s | rating: %.1f (%d) | downloads: %d", pkg.Category, pkg.Rating, pkg.RatingCount, pkg.Downloads))
		fmt.Println("     " + ui.Dim.Sprint(fmt.Sprintf("id: %s", pkg.ID)))
		fmt.Println()
	}
	return nil
}

func runStoreSearch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: anyclaw store search <keyword>")
	}
	keyword := strings.Join(args, " ")

	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		return err
	}

	results := sm.Search(keyword)
	if len(results) == 0 {
		fmt.Printf("No results for %q.\n", keyword)
		return nil
	}

	fmt.Println(ui.Bold.Sprint(fmt.Sprintf("Results (%d):", len(results))))
	fmt.Println()
	for _, pkg := range results {
		icon := pkg.Icon
		if icon == "" {
			icon = "-"
		}
		installed := ""
		if sm.IsInstalled(pkg.ID) {
			installed = ui.Green.Sprint(" [installed]")
		}
		fmt.Println("  " + icon + " " + ui.Bold.Sprint(pkg.DisplayName) + installed)
		fmt.Println("     " + ui.Dim.Sprint(pkg.Description))
		fmt.Println("     " + ui.Dim.Sprint(fmt.Sprintf("install: anyclaw store install %s", pkg.ID)))
		fmt.Println()
	}
	return nil
}

func runStoreInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: anyclaw store info <id>")
	}

	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		return err
	}

	pkg, err := sm.Get(args[0])
	if err != nil {
		return err
	}

	icon := pkg.Icon
	if icon == "" {
		icon = "-"
	}

	fmt.Println(icon + " " + ui.Bold.Sprint(pkg.DisplayName))
	fmt.Println()
	fmt.Println(fmt.Sprintf("  id:          %s", pkg.ID))
	fmt.Println(fmt.Sprintf("  description: %s", pkg.Description))
	fmt.Println(fmt.Sprintf("  author:      %s", pkg.Author))
	fmt.Println(fmt.Sprintf("  version:     %s", pkg.Version))
	fmt.Println(fmt.Sprintf("  category:    %s", pkg.Category))
	fmt.Println(fmt.Sprintf("  tags:        %s", strings.Join(pkg.Tags, ", ")))
	fmt.Println(fmt.Sprintf("  domain:      %s", pkg.Domain))
	fmt.Println(fmt.Sprintf("  expertise:   %s", strings.Join(pkg.Expertise, ", ")))
	fmt.Println(fmt.Sprintf("  skills:      %s", strings.Join(pkg.Skills, ", ")))
	fmt.Println(fmt.Sprintf("  permission:  %s", pkg.Permission))
	fmt.Println(fmt.Sprintf("  rating:      %.1f (%d)", pkg.Rating, pkg.RatingCount))
	fmt.Println(fmt.Sprintf("  downloads:   %d", pkg.Downloads))
	fmt.Println(fmt.Sprintf("  tone:        %s", pkg.Tone))
	fmt.Println(fmt.Sprintf("  style:       %s", pkg.Style))
	fmt.Println()
	fmt.Println("  system prompt:")
	fmt.Println("    " + ui.Dim.Sprint(pkg.SystemPrompt))
	fmt.Println()
	if sm.IsInstalled(pkg.ID) {
		fmt.Println("  " + ui.Green.Sprint("installed"))
	} else {
		fmt.Println("  " + ui.Dim.Sprint(fmt.Sprintf("install: anyclaw store install %s", pkg.ID)))
	}
	return nil
}

func runStoreInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: anyclaw store install <id>")
	}

	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		return err
	}

	id := args[0]
	if sm.IsInstalled(id) {
		printInfo("Already installed: %s", id)
		return nil
	}

	if err := sm.Install(id); err != nil {
		return err
	}

	pkg, _ := sm.Get(id)
	if pkg != nil {
		printSuccess("Installed: %s (%s)", pkg.DisplayName, id)
	} else {
		printSuccess("Installed: %s", id)
	}
	return nil
}

func runStoreUninstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: anyclaw store uninstall <id>")
	}

	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		return err
	}

	if err := sm.Uninstall(args[0]); err != nil {
		return err
	}
	printSuccess("Uninstalled: %s", args[0])
	return nil
}
