package input

import (
	"bufio"
	"context"
	"os"
)

func ReadLine(ctx context.Context) (line string, err error) {
	ch := make(chan struct{})

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line = scanner.Text()
			ch <- struct{}{}
			break
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ch:
			return
		}
	}
}
