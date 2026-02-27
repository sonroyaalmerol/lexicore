package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	serverAddr string
	apiPrefix  = "/apis/lexicore.io/v1"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "lexictl",
		Short: "lexictl controls the Lexicore identity orchestrator",
		Long:  `A command line tool to manage Lexicore IdentitySources and SyncTargets.`,
	}

	rootCmd.PersistentFlags().StringVarP(
		&serverAddr,
		"server", "s",
		"http://localhost:8080",
		"The address and port of the Lexicore API server",
	)

	rootCmd.AddCommand(newGetCommand())
	rootCmd.AddCommand(newReconcileCommand())
	rootCmd.AddCommand(newInspectCommand())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get [kind]",
		Short: "Display one or many resources",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			endpoint := getEndpoint(args[0])
			if endpoint == "" {
				fmt.Printf("Error: Unknown resource kind %q\n", args[0])
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
				meta, ok := item["metadata"].(map[string]any)
				if !ok {
					continue
				}

				status := "Active"
				if s, ok := item["status"].(map[string]any); ok {
					if st, ok := s["status"].(string); ok {
						status = st
					}
				}

				fmt.Fprintf(w, "%v\t%v\t%v\n", meta["name"], item["kind"], status)
			}
			w.Flush()
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
				resp, err := http.Post(
					serverAddr+apiPrefix+"/reconcile",
					"application/json",
					nil,
				)
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
					fmt.Printf(
						"⚠ Partial success: %v targets queued, %v failed\n",
						result["queued"],
						len(result["failed"].([]any)),
					)
					if failed, ok := result["failed"].([]any); ok {
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
			url := fmt.Sprintf(
				"%s%s/synctargets/%s/reconcile",
				serverAddr, apiPrefix, targetName,
			)

			resp, err := http.Post(url, "application/json", nil)
			if err != nil {
				fmt.Printf("Error connecting to server: %v\n", err)
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)

			var result map[string]any
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
			url := fmt.Sprintf(
				"%s%s/identitysources/%s/details",
				serverAddr, apiPrefix, sourceName,
			)

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

			if identitiesData, ok := result["identities"].(map[string]any); ok {
				fmt.Printf("Identities (%v):\n", identitiesData["count"])

				w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
				fmt.Fprintln(w, "UID\tUSERNAME\tEMAIL\tDISPLAY NAME\tGROUPS")

				if items, ok := identitiesData["items"].(map[string]any); ok {
					for uid, identity := range items {
						id, ok := identity.(map[string]any)
						if !ok {
							continue
						}

						groups := ""
						if groupList, ok := id["Groups"].([]any); ok && len(groupList) > 0 {
							groups = fmt.Sprintf("%d groups", len(groupList))
						}

						fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
							uid,
							getStringField(id, "Username"),
							getStringField(id, "Email"),
							getStringField(id, "DisplayName"),
							groups,
						)
					}
				}
				w.Flush()
				fmt.Println()
			}

			if groupsData, ok := result["groups"].(map[string]any); ok {
				fmt.Printf("Groups (%v):\n", groupsData["count"])

				w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
				fmt.Fprintln(w, "GID\tNAME\tDESCRIPTION\tMEMBERS")

				if items, ok := groupsData["items"].(map[string]any); ok {
					for gid, group := range items {
						grp, ok := group.(map[string]any)
						if !ok {
							continue
						}

						memberCount := 0
						if members, ok := grp["Members"].([]any); ok {
							memberCount = len(members)
						}

						fmt.Fprintf(w, "%s\t%s\t%s\t%d members\n",
							gid,
							getStringField(grp, "Name"),
							getStringField(grp, "Description"),
							memberCount,
						)
					}
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
