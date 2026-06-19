package phase

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

type stubImage struct {
	imgutil.Image
	result topLayerResult
	calls  int
}

type topLayerResult struct {
	sha string
	err error
}

func (s *stubImage) TopLayer() (string, error) {
	s.calls++
	return s.result.sha, s.result.err
}

// swapRetryTiming replaces the package-level retry timing vars with deterministic
// values for testing. It returns a pointer to the recorded sleep durations.
// Cleanup is handled via t.Cleanup.
func swapRetryTiming(t *testing.T, n int) *[]time.Duration {
	t.Helper()
	var recorded []time.Duration

	origDelays := topLayerDelays
	origSleep := topLayerSleep

	topLayerDelays = make([]time.Duration, n)
	for i := range topLayerDelays {
		topLayerDelays[i] = time.Duration(i+1) * time.Millisecond
	}
	topLayerSleep = func(d time.Duration) {
		recorded = append(recorded, d)
	}

	t.Cleanup(func() {
		topLayerDelays = origDelays
		topLayerSleep = origSleep
	})
	return &recorded
}

func TestOpenRemoteImage(t *testing.T) {
	spec.Run(t, "OpenRemoteImage", testOpenRemoteImage, spec.Report(report.Terminal{}))
}

func testOpenRemoteImage(t *testing.T, when spec.G, it spec.S) {
	var (
		logHandler *memory.Handler
		logger     *log.Logger
	)

	it.Before(func() {
		logHandler = memory.New()
		logger = &log.Logger{Handler: logHandler}
	})

	when("successful first attempt", func() {
		it("returns the image immediately with no retries", func() {
			sleeps := swapRetryTiming(t, 3)
			img := &stubImage{result: topLayerResult{sha: "sha-1"}}

			got, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				return img, nil
			})

			h.AssertNil(t, err)
			h.AssertEq(t, img.calls, 1)
			h.AssertEq(t, len(*sleeps), 0)
			// Verify the returned image is the one with the successful TopLayer
			sha, topLayerErr := got.TopLayer()
			h.AssertNil(t, topLayerErr)
			h.AssertEq(t, sha, "sha-1")
		})
	})

	when("transient failures", func() {
		it("succeeds after transient registry errors", func() {
			sleeps := swapRetryTiming(t, 4)
			var calls int
			results := []topLayerResult{
				{err: &transport.Error{StatusCode: http.StatusInternalServerError}},
				{err: &transport.Error{StatusCode: http.StatusInternalServerError}},
				{sha: "sha-1"},
			}

			got, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				idx := calls
				calls++
				return &stubImage{result: results[idx]}, nil
			})

			h.AssertNil(t, err)
			h.AssertEq(t, calls, 3)
			h.AssertEq(t, len(*sleeps), 2)
			sha, topLayerErr := got.TopLayer()
			h.AssertNil(t, topLayerErr)
			h.AssertEq(t, sha, "sha-1")
		})
	})

	when("non-retryable errors", func() {
		it("does not retry on 401 Unauthorized", func() {
			sleeps := swapRetryTiming(t, 4)
			var calls int

			_, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				calls++
				return &stubImage{result: topLayerResult{err: &transport.Error{StatusCode: http.StatusUnauthorized}}}, nil
			})

			h.AssertNotNil(t, err)
			var tErr *transport.Error
			h.AssertEq(t, errors.As(err, &tErr), true)
			h.AssertEq(t, tErr.StatusCode, http.StatusUnauthorized)
			h.AssertEq(t, calls, 1)
			h.AssertEq(t, len(*sleeps), 0)
		})

		it("does not retry on 403 Forbidden", func() {
			sleeps := swapRetryTiming(t, 4)
			var calls int

			_, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				calls++
				return &stubImage{result: topLayerResult{err: &transport.Error{StatusCode: http.StatusForbidden}}}, nil
			})

			h.AssertNotNil(t, err)
			var tErr *transport.Error
			h.AssertEq(t, errors.As(err, &tErr), true)
			h.AssertEq(t, tErr.StatusCode, http.StatusForbidden)
			h.AssertEq(t, calls, 1)
			h.AssertEq(t, len(*sleeps), 0)
		})
	})

	when("all retries exhausted", func() {
		it("returns the last error after all attempts fail", func() {
			sleeps := swapRetryTiming(t, 3)
			var calls int
			results := []topLayerResult{
				{err: errors.New("transient error 1")},
				{err: errors.New("transient error 2")},
				{err: errors.New("transient error 3")},
				{err: errors.New("final error")},
			}

			_, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				idx := calls
				calls++
				return &stubImage{result: results[idx]}, nil
			})

			h.AssertNotNil(t, err)
			h.AssertEq(t, calls, 4)
			h.AssertEq(t, len(*sleeps), 3)
		})
	})

	when("factory returns error", func() {
		it("retries when image creation fails", func() {
			sleeps := swapRetryTiming(t, 3)
			var calls int

			got, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				idx := calls
				calls++
				if idx < 2 {
					return nil, errors.New("connection refused")
				}
				return &stubImage{result: topLayerResult{sha: "sha-1"}}, nil
			})

			h.AssertNil(t, err)
			h.AssertEq(t, calls, 3)
			h.AssertEq(t, len(*sleeps), 2)
			sha, topLayerErr := got.TopLayer()
			h.AssertNil(t, topLayerErr)
			h.AssertEq(t, sha, "sha-1")
		})
	})

	when("non-retryable transport errors", func() {
		it("does not retry on 400 BadRequest", func() {
			sleeps := swapRetryTiming(t, 4)
			var calls int

			_, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				calls++
				return &stubImage{result: topLayerResult{err: &transport.Error{StatusCode: http.StatusBadRequest}}}, nil
			})

			h.AssertNotNil(t, err)
			var tErr *transport.Error
			h.AssertEq(t, errors.As(err, &tErr), true)
			h.AssertEq(t, tErr.StatusCode, http.StatusBadRequest)
			h.AssertEq(t, calls, 1)
			h.AssertEq(t, len(*sleeps), 0)
		})

		it("does not retry on 405 MethodNotAllowed", func() {
			sleeps := swapRetryTiming(t, 4)
			var calls int

			_, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				calls++
				return &stubImage{result: topLayerResult{err: &transport.Error{StatusCode: http.StatusMethodNotAllowed}}}, nil
			})

			h.AssertNotNil(t, err)
			var tErr *transport.Error
			h.AssertEq(t, errors.As(err, &tErr), true)
			h.AssertEq(t, tErr.StatusCode, http.StatusMethodNotAllowed)
			h.AssertEq(t, calls, 1)
			h.AssertEq(t, len(*sleeps), 0)
		})

		it("does not retry on 429 TooManyRequests", func() {
			sleeps := swapRetryTiming(t, 4)
			var calls int

			_, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				calls++
				return &stubImage{result: topLayerResult{err: &transport.Error{StatusCode: http.StatusTooManyRequests}}}, nil
			})

			h.AssertNotNil(t, err)
			var tErr *transport.Error
			h.AssertEq(t, errors.As(err, &tErr), true)
			h.AssertEq(t, tErr.StatusCode, http.StatusTooManyRequests)
			h.AssertEq(t, calls, 1)
			h.AssertEq(t, len(*sleeps), 0)
		})
	})

	when("retry logging", func() {
		it("logs warning for failed attempts and info on success", func() {
			_ = swapRetryTiming(t, 4)
			var calls int
			results := []topLayerResult{
				{err: &transport.Error{StatusCode: http.StatusInternalServerError}},
				{err: &transport.Error{StatusCode: http.StatusInternalServerError}},
				{sha: "sha-1"},
			}

			_, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				idx := calls
				calls++
				return &stubImage{result: results[idx]}, nil
			})

			h.AssertNil(t, err)
			h.AssertEq(t, calls, 3)

			var infoCount, warnCount int
			for _, entry := range logHandler.Entries {
				if entry.Level == log.InfoLevel {
					infoCount++
				}
				if entry.Level == log.WarnLevel {
					warnCount++
				}
			}
			h.AssertEq(t, infoCount, 1)
			h.AssertEq(t, warnCount, 2)
		})
	})

	when("zero retry delays configured", func() {
		it("makes a single attempt and returns the error", func() {
			sleeps := swapRetryTiming(t, 0)

			_, err := OpenRemoteImage(logger, func() (imgutil.Image, error) {
				return &stubImage{result: topLayerResult{err: errors.New("top layer error")}}, nil
			})

			h.AssertNotNil(t, err)
			h.AssertEq(t, len(*sleeps), 0)
		})
	})
}

func TestIsRetryable(t *testing.T) {
	spec.Run(t, "isRetryable", testIsRetryable, spec.Report(report.Terminal{}))
}

func testIsRetryable(t *testing.T, _ spec.G, it spec.S) {
	it("returns true for non-transport errors", func() {
		h.AssertEq(t, isRetryable(errors.New("generic error")), true)
	})

	it("returns true for retryable transport errors", func() {
		h.AssertEq(t, isRetryable(&transport.Error{StatusCode: http.StatusInternalServerError}), true)
		h.AssertEq(t, isRetryable(&transport.Error{StatusCode: http.StatusBadGateway}), true)
	})

	it("returns false for 400 BadRequest", func() {
		h.AssertEq(t, isRetryable(&transport.Error{StatusCode: http.StatusBadRequest}), false)
	})

	it("returns false for 401 Unauthorized", func() {
		h.AssertEq(t, isRetryable(&transport.Error{StatusCode: http.StatusUnauthorized}), false)
	})

	it("returns false for 403 Forbidden", func() {
		h.AssertEq(t, isRetryable(&transport.Error{StatusCode: http.StatusForbidden}), false)
	})

	it("returns false for 405 MethodNotAllowed", func() {
		h.AssertEq(t, isRetryable(&transport.Error{StatusCode: http.StatusMethodNotAllowed}), false)
	})

	it("returns false for 429 TooManyRequests", func() {
		h.AssertEq(t, isRetryable(&transport.Error{StatusCode: http.StatusTooManyRequests}), false)
	})
}
