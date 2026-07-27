package adapter

import (
	"slices"
	"testing"
)

func TestTokenizeMixed(t *testing.T) {
	tokens := tokenize("部署 Qdrant v1.10")
	// 中文 bigram + 英文/数字词(小写)。
	for _, want := range []string{"部署", "qdrant", "v1", "10"} {
		if !slices.Contains(tokens, want) {
			t.Errorf("tokens %v missing %q", tokens, want)
		}
	}
}

func TestTokenizeCJKBigram(t *testing.T) {
	tokens := tokenize("知识库")
	want := []string{"知识", "识库"}
	if !slices.Equal(tokens, want) {
		t.Errorf("got %v, want %v", tokens, want)
	}
}

func TestBuildSparseVectorEmpty(t *testing.T) {
	if sv := BuildSparseVector("!!! ---"); sv != nil {
		t.Errorf("expected nil for no-token text, got %v", sv)
	}
}

func TestBuildSparseVectorTF(t *testing.T) {
	sv := BuildSparseVector("go go rag")
	if sv == nil || len(sv.Indices) != 2 {
		t.Fatalf("expected 2 distinct tokens, got %v", sv)
	}
	goIdx := hashToken("go")
	for i, idx := range sv.Indices {
		if idx == goIdx && sv.Values[i] != 2 {
			t.Errorf("tf of 'go' = %v, want 2", sv.Values[i])
		}
	}
}

func TestFuseRRF(t *testing.T) {
	dense := []ScoredPoint{
		{ID: "a", Score: 0.9},
		{ID: "b", Score: 0.8},
	}
	sparse := []ScoredPoint{
		{ID: "b", Score: 12.3},
		{ID: "c", Score: 8.1},
	}
	out := fuseRRF(dense, sparse, 10)
	if len(out) != 3 {
		t.Fatalf("expected 3 fused points, got %d", len(out))
	}
	// b 同时命中两路,RRF 融合分最高。
	if out[0].ID != "b" {
		t.Errorf("top1 = %s, want b", out[0].ID)
	}
	for _, p := range out {
		switch p.ID {
		case "b":
			if p.Score != 0.8 {
				t.Errorf("b score = %v, want dense cosine 0.8", p.Score)
			}
		case "c":
			if p.Score != 0 {
				t.Errorf("sparse-only c score = %v, want 0", p.Score)
			}
		}
	}
}

func TestFuseRRFSparseEmpty(t *testing.T) {
	dense := []ScoredPoint{{ID: "a", Score: 0.9}}
	out := fuseRRF(dense, nil, 10)
	if len(out) != 1 || out[0].ID != "a" || out[0].Score != 0.9 {
		t.Errorf("expected passthrough of dense results, got %v", out)
	}
}
