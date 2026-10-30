package container

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func runAgainst(t *testing.T, handler http.HandlerFunc, name string) (bool, error, []UpdateProgress) {
	t.Helper()
	srv := httptest.NewServer(handler)
	defer srv.Close()
	t.Setenv("UPDATE_RUNNER_URL", srv.URL)
	t.Setenv("UPDATE_RUNNER_TOKEN", "tok")

	ch := make(chan UpdateProgress, 10)
	updated, err := runScript(context.Background(), name, ch)
	var got []UpdateProgress
	for p := range ch {
		got = append(got, p)
	}
	return updated, err, got
}

func TestRunScriptSuccess(t *testing.T) {
	updated, err, got := runAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/run/mygym" || r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		w.Write([]byte("building\nrestarting\n::exit 0\n"))
	}, "mygym")
	if err != nil || !updated {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
	if len(got) != 2 || got[0].Status != UpdateRecreating || got[1].Status != UpdateDone {
		t.Fatalf("progress = %+v", got)
	}
}

func TestRunScriptFailureKeepsTheTail(t *testing.T) {
	updated, err, got := runAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("step 1\nboom\n::exit 3\n"))
	}, "mygym")
	if updated || err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "exit 3") {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
	if last := got[len(got)-1]; last.Status != UpdateError || !strings.Contains(last.Error, "boom") {
		t.Fatalf("last progress = %+v", last)
	}
}

func TestRunScriptStreamEndingWithoutExitLineIsAFailure(t *testing.T) {
	updated, err, _ := runAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("half way\n"))
	}, "mygym")
	if updated || err == nil || !strings.Contains(err.Error(), "before the script finished") {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
}

func TestRunScriptUnknownNameAndNoRunner(t *testing.T) {
	updated, err, _ := runAgainst(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }, "nope")
	if updated || err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("updated=%v err=%v", updated, err)
	}

	t.Setenv("UPDATE_RUNNER_URL", "")
	ch := make(chan UpdateProgress, 2)
	if updated, err := runScript(context.Background(), "x", ch); updated || err == nil {
		t.Fatalf("no runner set: updated=%v err=%v", updated, err)
	}
}

func TestScriptName(t *testing.T) {
	if got := ScriptName(map[string]string{ScriptLabel: " mygym "}); got != "mygym" {
		t.Fatalf("got %q", got)
	}
	if ScriptName(nil) != "" {
		t.Fatal("no label should be empty")
	}
}
