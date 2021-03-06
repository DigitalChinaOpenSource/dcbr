// Copyright 2020 PingCAP, Inc. Licensed under Apache-2.0.

package restore

import (
	"time"

	"github.com/pingcap/errors"
	"github.com/pingcap/log"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	berrors "github.com/Orion7r/pr/pkg/errors"
	"github.com/Orion7r/pr/pkg/utils"
)

const (
	importSSTRetryTimes      = 16
	importSSTWaitInterval    = 10 * time.Millisecond
	importSSTMaxWaitInterval = 1 * time.Second

	downloadSSTRetryTimes      = 8
	downloadSSTWaitInterval    = 10 * time.Millisecond
	downloadSSTMaxWaitInterval = 1 * time.Second

	resetTSRetryTime       = 16
	resetTSWaitInterval    = 50 * time.Millisecond
	resetTSMaxWaitInterval = 500 * time.Millisecond
)

type importerBackoffer struct {
	attempt      int
	delayTime    time.Duration
	maxDelayTime time.Duration
}

// NewBackoffer creates a new controller regulating a truncated exponential backoff.
func NewBackoffer(attempt int, delayTime, maxDelayTime time.Duration) utils.Backoffer {
	return &importerBackoffer{
		attempt:      attempt,
		delayTime:    delayTime,
		maxDelayTime: maxDelayTime,
	}
}

func newImportSSTBackoffer() utils.Backoffer {
	return NewBackoffer(importSSTRetryTimes, importSSTWaitInterval, importSSTMaxWaitInterval)
}

func newDownloadSSTBackoffer() utils.Backoffer {
	return NewBackoffer(downloadSSTRetryTimes, downloadSSTWaitInterval, downloadSSTMaxWaitInterval)
}

func (bo *importerBackoffer) NextBackoff(err error) time.Duration {
	switch errors.Cause(err) { // nolint:errorlint
	case berrors.ErrKVEpochNotMatch, berrors.ErrKVDownloadFailed, berrors.ErrKVIngestFailed:
		bo.delayTime = 2 * bo.delayTime
		bo.attempt--
	case berrors.ErrKVRangeIsEmpty, berrors.ErrKVRewriteRuleNotFound:
		// Excepted error, finish the operation
		bo.delayTime = 0
		bo.attempt = 0
	default:
		switch status.Code(err) {
		case codes.Unavailable, codes.Aborted:
			bo.delayTime = 2 * bo.delayTime
			bo.attempt--
		default:
			// Unexcepted error
			bo.delayTime = 0
			bo.attempt = 0
			log.Warn("unexcepted error, stop to retry", zap.Error(err))
		}
	}
	if bo.delayTime > bo.maxDelayTime {
		return bo.maxDelayTime
	}
	return bo.delayTime
}

func (bo *importerBackoffer) Attempt() int {
	return bo.attempt
}

type pdReqBackoffer struct {
	attempt      int
	delayTime    time.Duration
	maxDelayTime time.Duration
}

func newPDReqBackoffer() utils.Backoffer {
	return &pdReqBackoffer{
		attempt:      resetTSRetryTime,
		delayTime:    resetTSWaitInterval,
		maxDelayTime: resetTSMaxWaitInterval,
	}
}

func (bo *pdReqBackoffer) NextBackoff(err error) time.Duration {
	bo.delayTime = 2 * bo.delayTime
	bo.attempt--
	if bo.delayTime > bo.maxDelayTime {
		return bo.maxDelayTime
	}
	return bo.delayTime
}

func (bo *pdReqBackoffer) Attempt() int {
	return bo.attempt
}
