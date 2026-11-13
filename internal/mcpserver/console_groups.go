package mcpserver

import (
	"slices"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// The console tools come in groups, so the Hermes relay can hand a run the core and only the
// domains its request needs (every schema rides in the model context on every step). Each tool of
// tools/list carries _meta {"simulator-ai/groups": [...]}; a tool may sit in several groups, and
// core is in every run. A group together with core is enough for its domain's skill.

// groupsMeta is the _meta key the relay filters on.
const groupsMeta = "simulator-ai/groups"

// consoleGroups lists each group's tools. core is the minimum of any request: read an actor or a
// template, find one, create or change one, comment, find a person, read a file, run a saved skill,
// link what was made. graph builds and reads a layer; graph_extra holds the graph tools a run rarely
// needs (holes, pencil drawings, opening a node, layer settings, the prune and compact engines), split
// out when graph_build would have taken graph past maxGroupBytes. A new group name needs sim-api's
// router and the Hermes relay to know it.
var consoleGroups = map[string][]string{
	"core": {"actor_get", "actors_search", "actor_by_ref_get", "actor_create", "actor_update", "form_get", "forms_list", "forms_count",
		"users_search", "reactions_list", "reaction_create", "attachment_read", "skills_find", "skill_get", "build_link"},
	"graph": {"actors_create", "actor_delete", "actor_status_update", "actor_system_get",
		"graph_get", "graph_list", "flowchart_blocks_list", "flowchart_layout", "graph_build", "graph_build_delete", "edge_types_list",
		"edge_create", "edges_create", "edge_get", "edge_update", "edge_delete", "edge_endpoints_update", "edge_exist", "edges_by_nodes_delete",
		"layer_create", "layer_get", "layer_placements_create", "layer_positions_update", "layer_viewport",
		"layer_placements_list", "layer_placements_all", "layer_placements_manage", "layer_actors_search", "layer_actors_move",
		"layer_clean", "layer_skeleton_get", "layer_stats_get", "layer_edges_list",
		"layer_element_exist", "layer_edge_settings_save", "hole_close"},
	// layer_stats_get is in graph too: a large build is verified by its counts.
	"graph_extra": {"hole_conditions_get", "hole_reopen", "edge_hole_close", "edge_hole_reopen", "layer_draw",
		"layer_node_expand", "layer_settings_update", "layer_stats_get", "layer_compact", "layer_prune_long_edges"},
	"smart_forms": {"smart_form_components", "smart_form_create", "smart_form_data_form_create", "smart_form_files_get",
		"smart_form_files_update", "smart_form_files_create", "smart_form_folders_create", "smart_form_release", "smart_form_release_get",
		"smart_form_releases_list", "smart_form_release_diff", "smart_form_release_rollback", "smart_form_file_versions_list",
		"smart_form_file_version_get", "smart_form_file_rollback", "smart_form_trash_list", "smart_form_trash_restore",
		"smart_form_page_render", "smart_form_page_submit", "corezoid_project_create", "corezoid_project_get",
		"smart_form_page_patch", "smart_form_reference_grep", "smart_form_reference_read"},
	"forms": {"form_create", "form_update", "form_fields_patch", "form_delete", "form_status_update", "forms_tree_get", "form_linked_get",
		"form_accounts_list", "form_account_create", "form_account_delete", "actors_filter", "actors_filter_count"},
	// finance is the everyday money of a request: an actor's accounts, a balance, a transfer, a
	// transaction, their history; finance_setup the rest, which shapes the ledger.
	"finance": {"account_create", "accounts_list", "account_get", "account_by_id_get", "account_pair_create",
		"currencies_list", "account_names_list", "transfer_create", "transfer_get", "transfer_by_ref_get", "transfers_filter",
		"transaction_create", "transaction_get", "transaction_by_ref_get", "transactions_list", "account_transactions_list",
		"transactions_filter", "actor_system_get"},
	"finance_setup": {"accounts_count", "account_update", "account_status_update", "account_delete", "account_by_ref_create",
		"account_by_ref_get", "accounts_by_ref_list", "account_pair_unregister", "account_amount_set", "account_formula_get",
		"account_formula_preview", "account_formula_set", "currency_get", "currency_create", "currency_update",
		"account_name_get", "account_name_create", "account_name_update", "transaction_by_actor_ref_create",
		"transactions_filter_count", "transaction_children_list", "atom_transaction_create", "transfers_filter_count",
		"counter_get", "counter_by_period_get", "counter_increment", "counter_set",
		"account_tags_list", "account_tag_add", "account_tags_set", "account_actor_tags_list", "account_actor_tags_count",
		"account_triggers_list", "account_triggers_set", "account_actor_triggers_list", "account_actor_triggers_set",
		"actor_field_triggers_list", "actor_field_triggers_set", "form_accounts_list", "form_account_create"},
	"people": {"access_rules_get", "access_rules_save", "access_requests_list", "access_request_create", "access_request_confirm",
		"access_request_cancel", "user_get", "workspaces_list", "search_all", "actor_system_get", "chat_open", "actor_attach",
		"reaction_update", "reaction_delete", "reactions_count", "reactions_mark_read", "agents_find", "agent_get", "bbcode_tags_get"},
	"files": {"attachment_upload", "attachment_attach", "attachment_detach", "attachment_update", "attachments_list",
		"attachments_by_actor_list", "actor_picture_upload"},
	"meetings": {"public_link_create", "public_link_get", "public_link_revoke", "meet_transcription_list"},
	"jobs":     {"graph_export_create", "graph_import_create", "graph_file_upload", "task_get"},
	// A chart plots accounts: their nameId and currencyId come from the finance reads; chart_data_get
	// reads what a chart shows over a period.
	"charts":     {"chart_create", "chart_update", "chart_data_get", "accounts_list", "account_names_list", "currencies_list"},
	"simulation": {"simulation_snapshot_get", "simulation_check", "simulation_run"},
	"corezoid": {"corezoid_projects_list", "corezoid_stages_list", "corezoid_processes_list", "corezoid_project_get",
		"corezoid_project_create", "corezoid_project_bind", "actor_corezoid_processes_list"},
}

// consoleCoreGroups are the groups whose union is the plain console catalog of the Console's
// everyday scenarios (consoleCoreTools).
var consoleCoreGroups = []string{"core", "graph", "graph_extra", "smart_forms", "forms", "finance", "people", "files"}

// Without the relay a run gets the plain console catalog (consoleCoreTools, 62 tools), not the
// union of these groups, which is larger (their setup tools, e.g. graph edits).

// toolGroups is each tool's groups, in the order of groupOrder.
var toolGroups = func() map[string][]string {
	out := map[string][]string{}
	for _, g := range groupOrder {
		for _, name := range consoleGroups[g] {
			if !slices.Contains(out[name], g) {
				out[name] = append(out[name], g)
			}
		}
	}
	return out
}()

var groupOrder = []string{"core", "graph", "graph_extra", "smart_forms", "forms", "finance", "finance_setup", "people", "files", "meetings", "jobs", "charts", "simulation", "corezoid"}

// tagGroups writes each console tool's groups into its _meta, keeping its handler.
func tagGroups(s *server.MCPServer) {
	for name, t := range s.ListTools() {
		groups := toolGroups[name]
		if len(groups) == 0 {
			continue // TestConsoleGroups fails for a tool without a group
		}
		tool := t.Tool
		meta := &mcp.Meta{AdditionalFields: map[string]any{}}
		if tool.Meta != nil {
			meta.ProgressToken = tool.Meta.ProgressToken
			for k, v := range tool.Meta.AdditionalFields {
				meta.AdditionalFields[k] = v
			}
		}
		meta.AdditionalFields[groupsMeta] = groups
		tool.Meta = meta
		s.AddTool(tool, t.Handler)
	}
}
