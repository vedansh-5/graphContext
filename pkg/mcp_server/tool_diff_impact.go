package mcp_server

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/vedansh-5/graphcontext/pkg/archlint"
	"github.com/vedansh-5/graphcontext/pkg/diff"
	"github.com/vedansh-5/graphcontext/pkg/testselect"
)

func registerDiffImpactTool(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("diff_impact",
		mcp.WithDescription("Change Intelligence: Evaluates a Git diff against the code graph to identify modified AST symbols, select affected tests, and detect architectural boundary violations."),
		mcp.WithString("project_path", mcp.Required(), mcp.Description("Absolute path of the project to analyze")),
		mcp.WithString("diff", mcp.Description("Optional raw unified diff text")),
		mcp.WithString("git_ref", mcp.Description("Optional git revision or range, e.g. HEAD~1 or main...HEAD")),
		mcp.WithBoolean("staged", mcp.Description("Whether to inspect staged git changes (--cached)")),
		mcp.WithString("rule_file", mcp.Description("Optional path to architectural boundary rules JSON file")),
	), withSession(handleDiffImpact))
}

func handleDiffImpact(sess *session, projectPath string, args map[string]any) (*mcp.CallToolResult, error) {
	rawDiff := argString(args, "diff")
	var fileDiffs []diff.FileDiff
	var err error

	if rawDiff != "" {
		fileDiffs, err = diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
	} else {
		var gitArgs []string
		if argBool(args, "staged") {
			gitArgs = append(gitArgs, "--cached")
		}
		if ref := argString(args, "git_ref"); ref != "" {
			gitArgs = append(gitArgs, ref)
		}
		fileDiffs, err = diff.RunGitDiff(projectPath, gitArgs...)
	}

	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	changedSymbols, err := diff.MapDiffToSymbols(fileDiffs, sess.store)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	testRes := testselect.SelectTests(changedSymbols, sess.graph, 10)

	rulePath := argString(args, "rule_file")
	if rulePath == "" {
		candidates := []string{
			filepath.Join(projectPath, "arch_rules.json"),
			filepath.Join(projectPath, ".archrules.json"),
		}
		for _, c := range candidates {
			if _, statErr := os.Stat(c); statErr == nil {
				rulePath = c
				break
			}
		}
	}

	var violations []archlint.Violation
	if rulePath != "" {
		rules, ruleErr := archlint.LoadRulesFromFile(rulePath)
		if ruleErr == nil {
			violations = archlint.LintDiff(changedSymbols, sess.graph, rules)
		}
	}

	answer := map[string]any{
		"changed_symbols":          changedSymbols,
		"affected_tests":           testRes.AffectedTests,
		"test_files":               testRes.TestFiles,
		"total_tests_in_repo":      testRes.TotalTestsInRepo,
		"affected_tests_count":     testRes.AffectedTestsCount,
		"reduction_percent":        testRes.ReductionPercent,
		"architectural_violations": violations,
	}

	stats := map[string]any{
		"changed_symbols_count": len(changedSymbols),
		"affected_tests_count":  testRes.AffectedTestsCount,
		"test_files_count":      len(testRes.TestFiles),
		"violations_count":      len(violations),
	}

	return toolJSON(Envelope{
		Answer:  answer,
		Caveats: testRes.Caveats,
		Stats:   stats,
	}), nil
}
