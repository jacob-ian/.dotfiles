package ghctl

import (
	"encoding/json"
	"testing"
)

func TestOwner(t *testing.T) {
	human := actor{Login: "dan", Typename: "User"}
	bot := actor{Login: "g3rtrude", Typename: "Bot"}
	events := func(as ...actor) []struct {
		Actor actor `json:"actor"`
	} {
		out := make([]struct {
			Actor actor `json:"actor"`
		}, len(as))
		for i, a := range as {
			out[i].Actor = a
		}
		return out
	}
	tests := map[string]struct {
		node prSearchNode
		want string
	}{
		"human author is nobody's delegate": {
			node: prSearchNode{Author: human, TimelineItems: struct {
				Nodes []struct {
					Actor actor `json:"actor"`
				} `json:"nodes"`
			}{Nodes: events(actor{Login: "other", Typename: "User"})}},
		},
		"newest human actor wins": {
			node: prSearchNode{Author: bot, TimelineItems: struct {
				Nodes []struct {
					Actor actor `json:"actor"`
				} `json:"nodes"`
			}{Nodes: events(actor{Login: "earlier", Typename: "User"}, human)}},
			want: "dan",
		},
		"bot actors are skipped": {
			node: prSearchNode{Author: bot, TimelineItems: struct {
				Nodes []struct {
					Actor actor `json:"actor"`
				} `json:"nodes"`
			}{Nodes: events(human, bot, bot)}},
			want: "dan",
		},
		"assignee when only bots acted": {
			node: func() prSearchNode {
				n := prSearchNode{Author: bot}
				n.TimelineItems.Nodes = events(bot)
				n.Assignees.Nodes = append(n.Assignees.Nodes, struct {
					Login string `json:"login"`
				}{Login: "kory"})
				return n
			}(),
			want: "kory",
		},
		"bot nobody picked up": {node: prSearchNode{Author: bot}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tt.node.owner(); got != tt.want {
				t.Errorf("owner() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Decodes the response shape GitHub actually returns, so a renamed field or
// mistyped struct tag fails here rather than silently dropping the byline.
func TestSearchBlockDecodesLiveShape(t *testing.T) {
	raw := `{"prs": {"nodes": [
	  {"number": 3205, "title": "Add events", "isDraft": false, "headRefName": "gertrude/en-8616", "baseRefName": "main",
	   "author": {"login": "g3rtrude", "__typename": "Bot"},
	   "repository": {"nameWithOwner": "eucalyptusvc/protobufs"},
	   "assignees": {"nodes": []},
	   "timelineItems": {"nodes": [
	     {"actor": {"login": "g3rtrude", "__typename": "Bot"}},
	     {"actor": {"login": "danielmolloy-euc", "__typename": "User"}}
	   ]}},
	  {"number": 1, "title": "Human", "author": {"login": "octocat", "__typename": "User"},
	   "repository": {"nameWithOwner": "eucalyptusvc/x"}, "timelineItems": {"nodes": [{"actor": null}]}}
	]}}`
	resp := map[string]prSearchBlock{}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	nodes := resp["prs"].Nodes
	if len(nodes) != 2 {
		t.Fatalf("decoded %d nodes, want 2", len(nodes))
	}
	if got := nodes[0].owner(); got != "danielmolloy-euc" {
		t.Errorf("bot PR owner = %q, want danielmolloy-euc", got)
	}
	if nodes[0].HeadRefName != "gertrude/en-8616" || nodes[0].Repository.NameWithOwner != "eucalyptusvc/protobufs" {
		t.Errorf("scalar fields not decoded: %+v", nodes[0])
	}
	if got := nodes[1].owner(); got != "" {
		t.Errorf("human PR owner = %q, want empty", got)
	}
}
