package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	serverAddr string
	apiPrefix  = "/apis/lexicore.io/v1"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "lexictl",
		Short: "lexictl controls the Lexicore identity orchestrator",
		Long:  `A command line tool to manage Lexicore IdentitySources and SyncTargets.`,
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "http://localhost:8080", "The address and port of the Lexicore API server")

	rootCmd.AddCommand(newApplyCommand())
	rootCmd.AddCommand(newGetCommand())
	rootCmd.AddCommand(newDeleteCommand())
	rootCmd.AddCommand(newReconcileCommand())
	rootCmd.AddCommand(newInspectCommand())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newApplyCommand() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a configuration to a resource by file name",
		Run: func(cmd *cobra.Command, args []string) {
			if file == "" {
				fmt.Println("Error: must specify -f <file>")
				return
			}

			data, err := os.ReadFile(file)
			if err != nil {
				fmt.Printf("Error reading file: %v\n", err)
				return
			}

			var base struct {
				Kind     string `yaml:"kind"`
				Metadata struct {
					Name string `yaml:"name"`
				} `yaml:"metadata"`
			}
			if err := yaml.Unmarshal(data, &base); err != nil {
				fmt.Printf("Error parsing YAML: %v\n", err)
				return
			}

			endpoint := getEndpoint(base.Kind)
			if endpoint == "" {
				fmt.Printf("Error: Unknown kind %q\n", base.Kind)
				return
			}

			var raw any
			yaml.Unmarshal(data, &raw)
			jsonBody, _ := json.Marshal(raw)

			resp, err := http.Post(serverAddr+apiPrefix+endpoint, "application/json", bytes.NewBuffer(jsonBody))
			if err != nil {
				fmt.Printf("Error connecting to server: %v\n", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				fmt.Printf("Error from server (%d): %s\n", resp.StatusCode, string(body))
				return
			}

			fmt.Printf("%s/%s applied\n", base.Kind, base.Metadata.Name)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Configuration file to apply")
	return cmd
}

func newGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get [kind]",
		Short: "Display one or many resources",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			kind := args[0]
			endpoint := getEndpoint(kind)
			if endpoint == "" {
				fmt.Printf("Error: Unknown resource kind %q\n", kind)
				return
			}

			resp, err := http.Get(serverAddr + apiPrefix + endpoint)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				fmt.Printf("Error: Server returned %d\n", resp.StatusCode)
				return
			}

			var items []map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
				fmt.Printf("Error decoding server response: %v\n", err)
				return
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
			fmt.Fprintln(w, "NAME\tKIND\tSTATUS")
			for _, item := range items {
				meta := item["metadata"].(map[string]any)
				name := meta["name"]

				status := "Active"
				if s, ok := item["status"].(map[string]any); ok {
					if st, ok := s["status"].(string); ok {
						status = st
					}
				}

				fmt.Fprintf(w, "%v\t%v\t%v\n", name, item["kind"], status)
			}
			w.Flush()
		},
	}
}

func newDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [kind] [name]",
		Short: "Delete resources by resources and names",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			kind := args[0]
			name := args[1]
			endpoint := getEndpoint(kind)
			if endpoint == "" {
				fmt.Printf("Error: Unknown kind %q\n", kind)
				return
			}

			req, _ := http.NewRequest(http.MethodDelete, serverAddr+apiPrefix+endpoint+"/"+name, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				fmt.Printf("%s %q deleted\n", kind, name)
			} else {
				body, _ := io.ReadAll(resp.Body)
				fmt.Printf("Failed to delete (Status: %d): %s\n", resp.StatusCode, string(body))
			}
		},
	}
}

func newReconcileCommand() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "reconcile [synctarget-name]",
		Short: "Manually trigger reconciliation for a SyncTarget or all SyncTargets",
		Long: `Trigger immediate reconciliation for a specific SyncTarget by name, 
or use --all to reconcile all SyncTargets at once.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if all {
				resp, err := http.Post(serverAddr+apiPrefix+"/reconcile", "application/json", nil)
				if err != nil {
					fmt.Printf("Error connecting to server: %v\n", err)
					return
				}
				defer resp.Body.Close()

				var result map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					fmt.Printf("Error decoding response: %v\n", err)
					return
				}

				switch resp.StatusCode {
				case http.StatusAccepted:
					fmt.Printf("✓ Reconciliation queued for %v targets\n", result["count"])
				case http.StatusPartialContent:
					fmt.Printf("⚠ Partial success: %v targets queued, %v failed\n",
						result["queued"], len(result["failed"].([]any)))
					if failed, ok := result["failed"].([]any); ok && len(failed) > 0 {
						fmt.Println("Failed targets:")
						for _, f := range failed {
							fmt.Printf("  - %v\n", f)
						}
					}
				default:
					body, _ := io.ReadAll(resp.Body)
					fmt.Printf("Error from server (%d): %s\n", resp.StatusCode, string(body))
				}
				return
			}

			if len(args) == 0 {
				fmt.Println("Error: must specify a SyncTarget name or use --all flag")
				cmd.Usage()
				return
			}

			targetName := args[0]
			url := fmt.Sprintf("%s%s/synctargets/%s/reconcile", serverAddr, apiPrefix, targetName)

			resp, err := http.Post(url, "application/json", nil)
			if err != nil {
				fmt.Printf("Error connecting to server: %v\n", err)
				return
			}
			defer resp.Body.Close()

			var result map[string]any
			body, _ := io.ReadAll(resp.Body)
			json.Unmarshal(body, &result)

			switch resp.StatusCode {
			case http.StatusAccepted:
				fmt.Printf("✓ Reconciliation queued for SyncTarget %q\n", targetName)
			case http.StatusInternalServerError:
				if errMsg, ok := result["error"].(string); ok {
					fmt.Printf("Error: %s\n", errMsg)
				} else {
					fmt.Printf("Error from server (%d): %s\n", resp.StatusCode, string(body))
				}
			default:
				fmt.Printf("Unexpected response (%d): %s\n", resp.StatusCode, string(body))
			}
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Trigger reconciliation for all SyncTargets")

	return cmd
}

func newInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect identitysource [name]",
		Short: "Inspect details of an identity source",
		Long:  "Display all identities and groups from a specific identity source",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if args[0] != "identitysource" {
				fmt.Printf("Error: only 'identitysource' is supported for inspection\n")
				return
			}

			sourceName := args[1]
			url := fmt.Sprintf("%s%s/identitysources/%s/details", serverAddr, apiPrefix, sourceName)

			resp, err := http.Get(url)
			if err != nil {
				fmt.Printf("Error connecting to server: %v\n", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				fmt.Printf("Error from server (%d): %s\n", resp.StatusCode, string(body))
				return
			}

			var result map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				fmt.Printf("Error decoding response: %v\n", err)
				return
			}

			fmt.Printf("\n=== Identity Source: %s ===\n\n", sourceName)

			// Display identities
			if identitiesData, ok := result["identities"].(map[string]any); ok {
				count := identitiesData["count"]
				items := identitiesData["items"].(map[string]any)

				fmt.Printf("Identities (%v):\n", count)
				w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
				fmt.Fprintln(w, "UID\tUSERNAME\tEMAIL\tDISPLAY NAME\tGROUPS")

				for uid, identity := range items {
					id := identity.(map[string]any)
					username := getStringField(id, "Username")
					email := getStringField(id, "Email")
					displayName := getStringField(id, "DisplayName")

					groups := ""
					if groupList, ok := id["Groups"].([]any); ok && len(groupList) > 0 {
						groupStrs := make([]string, len(groupList))
						for i, g := range groupList {
							groupStrs[i] = fmt.Sprintf("%v", g)
						}
						groups = fmt.Sprintf("%d groups", len(groupStrs))
					}

					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", uid, username, email, displayName, groups)
				}
				w.Flush()
				fmt.Println()
			}

			// Display groups
			if groupsData, ok := result["groups"].(map[string]any); ok {
				count := groupsData["count"]
				items := groupsData["items"].(map[string]any)

				fmt.Printf("Groups (%v):\n", count)
				w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
				fmt.Fprintln(w, "GID\tNAME\tDESCRIPTION\tMEMBERS")

				for gid, group := range items {
					grp := group.(map[string]any)
					name := getStringField(grp, "Name")
					desc := getStringField(grp, "Description")

					memberCount := 0
					if members, ok := grp["Members"].([]any); ok {
						memberCount = len(members)
					}

					fmt.Fprintf(w, "%s\t%s\t%s\t%d members\n", gid, name, desc, memberCount)
				}
				w.Flush()
			}
		},
	}
}

func getEndpoint(kind string) string {
	switch kind {
	case "IdentitySource", "identitysource", "is", "identitysources":
		return "/identitysources"
	case "SyncTarget", "synctarget", "st", "synctargets":
		return "/synctargets"
	default:
		return ""
	}
}

func getStringField(m map[string]any, field string) string {
	if val, ok := m[field]; ok {
		return fmt.Sprintf("%v", val)
	}
	return ""
}
