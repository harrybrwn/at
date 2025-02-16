package xrpc

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestSubscription(t *testing.T) {
	// ctx := t.Context()
	// c, _ := websocket.Accept(nil, nil, nil)
	// w, _ := c.Writer(ctx, websocket.MessageText)
}

func generate[T any](d time.Duration, vals []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, v := range vals {
			if !yield(v) {
				return
			}
			time.Sleep(d)
		}
	}
}

func TestWebsockets(t *testing.T) {
	go func() {
		_ = http.ListenAndServe(":9998", http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			ctx := r.Context()
			c, err := websocket.Accept(w, r, nil)
			if err != nil {
				log.Fatal(err)
			}
			defer c.Close(websocket.StatusGoingAway, "")
			err = Stream(ctx, c, generate(time.Second, []map[string]any{
				{"one": 1, "two": 2, "three": 3},
				{"a": "A", "b": "B", "c": "C"},
				{
					"things":  map[string]any{"one": 1, "two": 2, "three": 3},
					"letters": map[string]any{"a": "A", "b": "B", "c": "C"},
				},
			}))
			if err != nil {
				log.Fatal(err)
			}
		}))
	}()

	ctx := t.Context()
	c, _, err := websocket.Dial(ctx, "ws://localhost:9998", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer c.CloseNow()
	for {
		_, r, err := c.Reader(ctx)
		// if errors.Is(err, websocket.StatusInternalError) {
		// 	break
		// }
		if err != nil {
			fmt.Printf("%#v\n", err)
			fmt.Printf("%#v\n", errors.Unwrap(err))
			var closeErr websocket.CloseError
			if errors.As(err, &closeErr) {
				fmt.Println("got close")
				break
			} else {
				log.Fatal(err)
			}
		}
		_, err = io.Copy(os.Stdout, r)
		if err != nil {
			log.Fatal(err)
		}
	}
}
