package bootstrap

import (
	"context"
	"testing"

	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/transport"
)

type staticStrategy struct{ result *Result }

func (s staticStrategy) Bootstrap(context.Context, []string) (*Result, error) { return s.result, nil }

// pagedTransport serves one change per FetchUpdate call, like the server does.
type pagedTransport struct {
	transport.Transport
	pages map[string]*model.UpdateFetchResponse // keyed by request cursor
	calls int
}

func (p *pagedTransport) FetchUpdate(_ context.Context, req *model.UpdateFetchRequest) (*model.UpdateFetchResponse, error) {
	p.calls++
	if r, ok := p.pages[req.Cursor]; ok {
		return r, nil
	}
	return &model.UpdateFetchResponse{Cursor: req.Cursor}, nil
}

func family(version string) model.FigFamily {
	v := version
	return model.FigFamily{
		Definition:     model.FigDefinition{Namespace: "ns", Key: "k", SchemaURI: "u"},
		Figs:           []model.Fig{{Version: version}},
		DefaultVersion: &v,
	}
}

func TestHybridStrategy_CatchUpFollowsAllPages(t *testing.T) {
	backup := staticStrategy{&Result{
		FigFamilies: []model.FigFamily{family("v2")},
		Cursors:     map[string]string{"ns": "c0"},
		Schemas:     map[string]string{"u": "schema"},
	}}
	tr := &pagedTransport{pages: map[string]*model.UpdateFetchResponse{
		"c0": {Cursor: "c1", FigFamilies: []model.FigFamily{family("v3")}},
		"c1": {Cursor: "c2", FigFamilies: []model.FigFamily{family("v4")}},
	}}

	res, err := NewHybridStrategy(backup, nil, tr, "env").Bootstrap(context.Background(), []string{"ns"})
	if err != nil {
		t.Fatal(err)
	}
	last := res.FigFamilies[len(res.FigFamilies)-1]
	if *last.DefaultVersion != "v4" {
		t.Fatalf("last family version = %s, want v4 (catch-up stopped early)", *last.DefaultVersion)
	}
	if res.Cursors["ns"] != "c2" {
		t.Fatalf("cursor = %s, want c2", res.Cursors["ns"])
	}
	if res.Schemas["u"] != "schema" {
		t.Fatalf("backup schemas dropped: %v", res.Schemas)
	}
	if tr.calls != 3 {
		t.Fatalf("FetchUpdate calls = %d, want 3", tr.calls)
	}
}
