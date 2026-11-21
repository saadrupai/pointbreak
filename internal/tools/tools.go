// Package tools exposes workspace operations as MCP tools.
package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/saadrupai/pointbreak/internal/mcp"
	"github.com/saadrupai/pointbreak/internal/workspace"
)

// Every tool spells out all four MCP annotations: writes touch only context/ files and nothing is
// deleted (end_task moves a task into the log), so no tool is destructive or open-world.

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func strs(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, fmt.Errorf("invalid arguments: %v", err)
	}
	return v, nil
}

// All returns every Pointbreak tool bound to a workspace.
func All(ws *workspace.Workspace) []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "init_workspace",
			Description: "Create the context/ folder: team rules, system map, and per-service temp.md, architecture.md, features.md, database.md and this month's log. With no services, every git repo in the workspace root is registered. Never overwrites existing files.",
			InputSchema: obj(map[string]any{"services": strs("Service repo directory names. Omit to register every git repo in the workspace root.")}),
			Annotations: mcp.Annotations{Title: "Set up context folder", ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
			Handler: func(raw json.RawMessage) (string, error) {
				a, err := decode[struct {
					Services []string `json:"services"`
				}](raw)
				if err != nil {
					return "", err
				}
				created, err := ws.Init(a.Services)
				if err != nil {
					return "", err
				}
				if len(created) == 0 {
					return "Everything already exists; nothing created.", nil
				}
				return fmt.Sprintf("Created %d files:\n- %s\nNext: fill each service's architecture.md, features.md and database.md from its code, and the team settings in context/README.md.", len(created), strings.Join(created, "\n- ")), nil
			},
		},
		{
			Name:        "list_services",
			Description: "List registered services (with newest log month) and git repos in the workspace that have no context yet.",
			InputSchema: obj(map[string]any{}),
			Annotations: mcp.Annotations{Title: "List services", ReadOnlyHint: true, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
			Handler:     func(json.RawMessage) (string, error) { return ws.ListServices() },
		},
		{
			Name:        "load_context",
			Description: "Load the memory needed to work on one or more services in one call, in a fixed order: team rules, system map (automatic for 2+ services), then per service architecture, database, features, today's temp.md and the newest monthly log. A service whose repo has commits newer than its docs gets a 'docs may be out of date' warning first. Use skip_rules when context/README.md is already imported in CLAUDE.md, and files to load only what the task needs.",
			InputSchema: obj(map[string]any{
				"services":          strs("Service names, e.g. [\"orders-svc\", \"payments-svc\"]."),
				"files":             strs("Subset of: architecture.md, database.md, features.md, temp.md, log. Omit for all."),
				"include_global":    map[string]any{"type": "boolean", "description": "Force the system map on or off. Default: on for 2+ services."},
				"skip_rules":        map[string]any{"type": "boolean", "description": "Skip context/README.md (already in CLAUDE.md)."},
				"include_standards": map[string]any{"type": "boolean", "description": "Add context/STANDARDS.md (TDD, clean code, contracts, migrations, security…). Use when planning or writing code; skip for questions."},
			}, "services"),
			Annotations: mcp.Annotations{Title: "Load service context", ReadOnlyHint: true, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
			Handler: func(raw json.RawMessage) (string, error) {
				a, err := decode[struct {
					Services      []string `json:"services"`
					Files         []string `json:"files"`
					IncludeGlobal *bool    `json:"include_global"`
					SkipRules     bool     `json:"skip_rules"`
					Standards     bool     `json:"include_standards"`
				}](raw)
				if err != nil {
					return "", err
				}
				return ws.LoadContext(workspace.LoadOptions{Services: a.Services, Files: a.Files, IncludeGlobal: a.IncludeGlobal, SkipRules: a.SkipRules, Standards: a.Standards})
			},
		},
		{
			Name:        "start_task",
			Description: "Start a task in a service: adds today's '### branch: <branch> — <title>' block to its temp.md with ticket, inputs, related services and goal. The branch defaults to the repo's current git branch. Unfinished entries from earlier days are moved to the logs first. Call once per service involved.",
			InputSchema: obj(map[string]any{
				"service":      str("Service name."),
				"title":        str("Short task title."),
				"branch":       str("Branch for this service. Omit to read the repo's current branch."),
				"goal":         str("One or two sentences: what done looks like."),
				"ticket":       str("Ticket key or URL."),
				"inputs":       strs("Docs, designs, specs and prompts the task is based on."),
				"also_touches": strs("Other services in this task."),
			}, "service", "title"),
			Annotations: mcp.Annotations{Title: "Start task", ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: false, OpenWorldHint: false},
			Handler: func(raw json.RawMessage) (string, error) {
				a, err := decode[struct {
					Service     string   `json:"service"`
					Title       string   `json:"title"`
					Branch      string   `json:"branch"`
					Goal        string   `json:"goal"`
					Ticket      string   `json:"ticket"`
					Inputs      []string `json:"inputs"`
					AlsoTouches []string `json:"also_touches"`
				}](raw)
				if err != nil {
					return "", err
				}
				return ws.StartTask(workspace.StartInput{Service: a.Service, Title: a.Title, Branch: a.Branch, Goal: a.Goal, Ticket: a.Ticket, Inputs: a.Inputs, AlsoTouches: a.AlsoTouches})
			},
		},
		{
			Name:        "save_progress",
			Description: "Checkpoint (the /pointbreak save step): append a timestamped Plan / Done / Next / Decisions / Open questions update to one of today's tasks in a service's temp.md. Multi-line text becomes a nested list.",
			InputSchema: obj(map[string]any{
				"service":   str("Service name."),
				"title":     str("Which of today's tasks (part of its title). Omit for the latest."),
				"plan":      str("The plan, or plan changes."),
				"done":      str("What is finished."),
				"next":      str("The next step."),
				"decisions": str("Decisions made and why."),
				"questions": str("Open questions."),
			}, "service"),
			Annotations: mcp.Annotations{Title: "Save progress", ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: false, OpenWorldHint: false},
			Handler: func(raw json.RawMessage) (string, error) {
				a, err := decode[struct {
					Service, Title, Plan, Done, Next, Decisions, Questions string
				}](raw)
				if err != nil {
					return "", err
				}
				return ws.SaveProgress(workspace.SaveInput{Service: a.Service, Title: a.Title, Plan: a.Plan, Done: a.Done, Next: a.Next, Decisions: a.Decisions, Questions: a.Questions})
			},
		},
		{
			Name:        "end_task",
			Description: "Finish a task that is fully done, in every service it touched: write it to the top of each service's monthly log (a short summary first, then all the details from temp.md: goal, inputs, plan, checkpoints, decisions) and clear it from each temp.md, which returns to '(none active)' when no task is left for today. Update architecture/features/database docs separately if the work changed them.",
			InputSchema: obj(map[string]any{
				"services": strs("Every service the task touched. Use this for multi-service tasks."),
				"service":  str("A single service (same as services with one name)."),
				"title":    str("Which of today's tasks (part of its title). Omit for the latest."),
				"summary":  strs("Short summary bullets: what changed, where, why, status. All details from temp.md are kept below them."),
			}),
			Annotations: mcp.Annotations{Title: "End task", ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: false, OpenWorldHint: false},
			Handler: func(raw json.RawMessage) (string, error) {
				a, err := decode[struct {
					Service  string   `json:"service"`
					Services []string `json:"services"`
					Title    string   `json:"title"`
					Summary  []string `json:"summary"`
				}](raw)
				if err != nil {
					return "", err
				}
				svcs := a.Services
				if a.Service != "" {
					svcs = append([]string{a.Service}, svcs...)
				}
				if len(svcs) == 0 {
					return "", fmt.Errorf("pass services (every service the task touched) or service")
				}
				return ws.EndTaskIn(svcs, a.Title, a.Summary)
			},
		},
		{
			Name:        "task_status",
			Description: "Show today's active tasks across all services, with each task's latest Next step, and flag stale entries from earlier days. Also lists services whose docs may be out of date (repo commits newer than the docs).",
			InputSchema: obj(map[string]any{}),
			Annotations: mcp.Annotations{Title: "Task status", ReadOnlyHint: true, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
			Handler:     func(json.RawMessage) (string, error) { return ws.Status() },
		},
		{
			Name:        "search_logs",
			Description: "Search past work in the monthly logs (case-insensitive), newest first, with service, date and task for each match. Use it to find how something was done before or why it changed.",
			InputSchema: obj(map[string]any{
				"query":    str("Text to find: a ticket key, file, table, event, word."),
				"services": strs("Limit to these services. Omit for all."),
				"limit":    map[string]any{"type": "integer", "description": "Max matches (default 30, max 200)."},
			}, "query"),
			Annotations: mcp.Annotations{Title: "Search work logs", ReadOnlyHint: true, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
			Handler: func(raw json.RawMessage) (string, error) {
				a, err := decode[struct {
					Query    string   `json:"query"`
					Services []string `json:"services"`
					Limit    int      `json:"limit"`
				}](raw)
				if err != nil {
					return "", err
				}
				return ws.SearchLogs(a.Query, a.Services, a.Limit)
			},
		},
		{
			Name:        "sync_database",
			Description: "Rebuild a database doc from migrations (golang-migrate, goose, plain SQL or a Prisma schema), read-only: tables, columns, types, nulls, defaults, keys, indexes, foreign keys, enums and a migration log, written between generated markers so human-written sections stay. Own database: pass service (migrations are found in its repo). Shared database: pass database, source and users (or [\"*\"] for every service); the schema goes to context/_db/<database>.md and each user service's database.md gets a block linking it, with the tables its code references.",
			InputSchema: obj(map[string]any{
				"service":          str("Service with its own database."),
				"database":         str("Name of a database shared by several services, e.g. core."),
				"source":           str("Migrations folder or schema.prisma, relative to the workspace root. Optional for own databases (found in the repo); required for shared."),
				"users":            strs("Services using the shared database. [\"*\"] = all services. Optional after the first sync."),
				"dry_run":          map[string]any{"type": "boolean", "description": "Preview the generated schema without writing."},
				"replace_sections": map[string]any{"type": "boolean", "description": "For a hand-written doc without generated markers: replace only its Tables, Relations, Enums and Migration log sections."},
			}),
			Annotations: mcp.Annotations{Title: "Sync database doc from migrations", ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
			Handler: func(raw json.RawMessage) (string, error) {
				a, err := decode[struct {
					Service         string   `json:"service"`
					Database        string   `json:"database"`
					Source          string   `json:"source"`
					Users           []string `json:"users"`
					DryRun          bool     `json:"dry_run"`
					ReplaceSections bool     `json:"replace_sections"`
				}](raw)
				if err != nil {
					return "", err
				}
				return ws.SyncDatabase(workspace.SyncInput{Service: a.Service, Database: a.Database, Source: a.Source, Users: a.Users, DryRun: a.DryRun, ReplaceSections: a.ReplaceSections})
			},
		},
		{
			Name:        "impact",
			Description: "Before changing a contract, find everything that depends on it: rows in the system map (calls, events, queues, with their section) and lines in every service's architecture, database and features docs that mention the term. Pass an event subject, RPC, endpoint, queue, table or field. It searches docs, not code: confirm consumers in code, and it lists services whose docs may be out of date.",
			InputSchema: obj(map[string]any{"term": str("e.g. order.created, Payments.Refund, refunds, notify:send.")}, "term"),
			Annotations: mcp.Annotations{Title: "Find impact of a change", ReadOnlyHint: true, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
			Handler: func(raw json.RawMessage) (string, error) {
				a, err := decode[struct {
					Term string `json:"term"`
				}](raw)
				if err != nil {
					return "", err
				}
				return ws.Impact(a.Term)
			},
		},
	}
}
