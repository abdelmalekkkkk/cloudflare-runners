package input

import (
	"context"
	"os"
)

func WaitForEnter(ctx context.Context) (err error) {
	ch := make(chan struct{})

	go func() {
		buf := make([]byte, 1)
		for {
			_, err = os.Stdin.Read(buf)
			if err != nil {
				return
			}
			ch <- struct{}{}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
			return
		}
	}
}
