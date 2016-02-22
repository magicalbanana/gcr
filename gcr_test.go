package gcr

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClearContextHandler(t *testing.T) {
	Start(nil)
	r, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	ranHandler := false
	var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		err := Set(r, "test", "testing")
		if err != nil {
			t.Fatal("failed the set: ", err.Error())
		}

		ctx := GetContext(r)
		if v, ok := ctx.Value("test").(string); !ok || v != "testing" {
			t.Fatal("failed the get value: ", err.Error())
		}

		err = SetContext(r, ctx)
		if err != nil {
			t.Fatal("failed to set context: ", err.Error())
		}

		v, err := Get(r, "test")
		if err != nil {
			t.Fatal("failed the get value: ", err.Error())
		}
		if vv, ok := v.(string); !ok || vv != "testing" {
			t.Fatal("failed the get value: ", err.Error())
		}
		ranHandler = true
	}

	clearHandler := NewClearContextHandler(handler)
	clearHandler.ServeHTTP(w, r)

	if !ranHandler {
		t.Fatal("failed to run handler: ")
	}

	vs, err := Get(r, "test")
	if err == nil {
		t.Fatal("Should have failed the get, context should have been clear ", vs)
	}
}

func BenchmarkSet(b *testing.B) {
	Start(nil)
	done := make(chan bool)
	r, _ := http.NewRequest("GET", "/", nil)
	for i := 0; i < b.N; i++ {
		go func() {
			Set(r, i, "testing")
			done <- true
		}()
	}
	for i := 0; i < b.N; i++ {
		<-done
	}
}

func BenchmarkGet(b *testing.B) {
	Start(nil)
	r, _ := http.NewRequest("GET", "/", nil)
	//	gcr.Set(r, "test", "testing")
	done := make(chan bool)
	for i := 0; i < b.N; i++ {
		go func() {
			GetContext(r)
			done <- true
		}()
	}
	for i := 0; i < b.N; i++ {
		<-done
	}
}
