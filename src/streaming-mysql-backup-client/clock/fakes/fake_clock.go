// Code generated by counterfeiter. DO NOT EDIT.
package fakes

import (
	"sync"
	"time"

	"github.com/cloudfoundry/streaming-mysql-backup-client/clock"
)

type FakeClock struct {
	AfterStub        func(time.Duration) <-chan time.Time
	afterMutex       sync.RWMutex
	afterArgsForCall []struct {
		arg1 time.Duration
	}
	afterReturns struct {
		result1 <-chan time.Time
	}
	afterReturnsOnCall map[int]struct {
		result1 <-chan time.Time
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *FakeClock) After(arg1 time.Duration) <-chan time.Time {
	fake.afterMutex.Lock()
	ret, specificReturn := fake.afterReturnsOnCall[len(fake.afterArgsForCall)]
	fake.afterArgsForCall = append(fake.afterArgsForCall, struct {
		arg1 time.Duration
	}{arg1})
	stub := fake.AfterStub
	fakeReturns := fake.afterReturns
	fake.recordInvocation("After", []interface{}{arg1})
	fake.afterMutex.Unlock()
	if stub != nil {
		return stub(arg1)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *FakeClock) AfterCallCount() int {
	fake.afterMutex.RLock()
	defer fake.afterMutex.RUnlock()
	return len(fake.afterArgsForCall)
}

func (fake *FakeClock) AfterCalls(stub func(time.Duration) <-chan time.Time) {
	fake.afterMutex.Lock()
	defer fake.afterMutex.Unlock()
	fake.AfterStub = stub
}

func (fake *FakeClock) AfterArgsForCall(i int) time.Duration {
	fake.afterMutex.RLock()
	defer fake.afterMutex.RUnlock()
	argsForCall := fake.afterArgsForCall[i]
	return argsForCall.arg1
}

func (fake *FakeClock) AfterReturns(result1 <-chan time.Time) {
	fake.afterMutex.Lock()
	defer fake.afterMutex.Unlock()
	fake.AfterStub = nil
	fake.afterReturns = struct {
		result1 <-chan time.Time
	}{result1}
}

func (fake *FakeClock) AfterReturnsOnCall(i int, result1 <-chan time.Time) {
	fake.afterMutex.Lock()
	defer fake.afterMutex.Unlock()
	fake.AfterStub = nil
	if fake.afterReturnsOnCall == nil {
		fake.afterReturnsOnCall = make(map[int]struct {
			result1 <-chan time.Time
		})
	}
	fake.afterReturnsOnCall[i] = struct {
		result1 <-chan time.Time
	}{result1}
}

func (fake *FakeClock) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.afterMutex.RLock()
	defer fake.afterMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *FakeClock) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ clock.Clock = new(FakeClock)
