package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func BenchmarkInMemoryQueueEnqueueDequeue(b *testing.B) {
	q := NewInMemoryQueue()
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		if err := q.Enqueue("job", PriorityNormal); err != nil {
			b.Fatalf("enqueue: %v", err)
		}
		if _, err := q.Dequeue(ctx); err != nil {
			b.Fatalf("dequeue: %v", err)
		}
	}
}

func BenchmarkInMemoryQueueParallelRoundTrip(b *testing.B) {
	q := NewInMemoryQueue()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := q.Enqueue("job", PriorityNormal); err != nil {
				panic(err)
			}
			if _, err := q.Dequeue(ctx); err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkInMemoryQueueBlockedWakeup(b *testing.B) {
	for _, workers := range []int{1, 8, 32, 64} {
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			q := NewInMemoryQueue()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			results := make(chan struct{}, workers*2)
			var wg sync.WaitGroup

			for range workers {
				wg.Go(func() {
					for {
						_, err := q.Dequeue(ctx)
						if err != nil {
							if ctx.Err() != nil || isQueueClosedError(err) {
								return
							}
							panic(err)
						}
						select {
						case results <- struct{}{}:
						case <-ctx.Done():
							return
						}
					}
				})
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := q.Enqueue("job", PriorityNormal); err != nil {
					b.Fatalf("enqueue: %v", err)
				}
				<-results
			}
			b.StopTimer()

			cancel()
			wg.Wait()
		})
	}
}
