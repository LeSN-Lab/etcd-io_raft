// Copyright 2019 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package quorum

/*
#include "./quorumC/majority.h"
*/
import "C"

import (
	"math"
	"unsafe"
)

// MajorityConfig is a set of IDs that uses majority quorums to make decisions.
type MajorityConfig map[uint64]struct{}

// by chanjun
func (c MajorityConfig) String() string {
	// make slice
	sl := make([]uint64, 0, len(c))

	// push key only
	for id := range c {
		sl = append(sl, id)
	}

	if len(sl) != 0 {
		return C.GoString(C.cMajorityConfig((unsafe.Pointer(&sl[0])), C.int(len(sl))))
	} else {
		return "()"
	}
}

// Describe returns a (multi-line) representation of the commit indexes for the
// given lookuper.

func (c MajorityConfig) Describe(l AckedIndexer) string {
	c_len := C.int(len(c)) // c_len 추출

	// c_range, l_range_idx, l_range_ok 추출
	var c_keys []uint64
	var l_idx []Index
	var l_ok []bool
	for k := range c {
		c_keys = append(c_keys, k)
		idx, ok := l.AckedIndex(k)
		l_idx = append(l_idx, idx)
		l_ok = append(l_ok, ok)
	}
	// c_keys, l_idx, l_ok를 C의 void*로 변환
	var c_range unsafe.Pointer
	var l_range_idx unsafe.Pointer
	var l_range_ok unsafe.Pointer
	if len(c) > 0 {
		c_range = unsafe.Pointer(&c_keys[0])
		l_range_idx = unsafe.Pointer(&l_idx[0])
		l_range_ok = unsafe.Pointer(&l_ok[0])
	}

	describe_c_ans := C.GoString(C.cDescribe(c_len, c_range, l_range_idx, l_range_ok))

	return describe_c_ans
}

// by chanjun
// Slice returns the MajorityConfig as a sorted slice.
func (c MajorityConfig) Slice() []uint64 {
	var sl []uint64
	for id := range c {
		sl = append(sl, id)
	}

	// 조건문이 없으면 "index out of range [0] with length 0"라는 오류가 생김
	// sort.Slice(sl, func(i, j int) bool { return sl[i] < sl[j] })
	if len(sl) != 0 {
		C.cSlice(unsafe.Pointer(&sl[0]), C.int(len(sl)))
	}

	return sl
}

// by chanjun
// insertionSort : just quick sort
func insertionSort(arr []uint64) {
	if len(arr) != 0 {
		// C 함수 호출
		C.cinsertionSort(unsafe.Pointer(&arr[0]), C.int(len(arr)))
	}
}

// CommittedIndex computes the committed index from those supplied via the
// provided AckedIndexer (for the active config).
// Majority Config c : id[i + 1], value[struct{}{}]
// AckedIndexer l : id[i + 1] value[Index(rand.Int63n(math.MaxInt64))
func (c MajorityConfig) CommittedIndex(l AckedIndexer) Index {
	n := len(c)
	if n == 0 {
		// This plays well with joint quorums which, when one half is the zero
		// MajorityConfig, should behave like the other half.
		return math.MaxUint64
	}

	// Use an on-stack slice to collect the committed indexes when n <= 7
	// (otherwise we alloc). The alternative is to stash a slice on
	// MajorityConfig, but this impairs usability (as is, MajorityConfig is just
	// a map, and that's nice). The assumption is that running with a
	// replication factor of >7 is rare, and in cases in which it happens
	// performance is a lesser concern (additionally the performance
	// implications of an allocation here are far from drastic).
	var stk [7]uint64
	var srt []uint64
	if len(stk) >= n {
		srt = stk[:n]
	} else {
		srt = make([]uint64, n)
	}

	{
		// Fill the slice with the indexes observed. Any unused slots will be
		// left as zero; these correspond to voters that may report in, but
		// haven't yet. We fill from the right (since the zeroes will end up on
		// the left after sorting below anyway).
		i := n - 1
		for id := range c {
			if idx, ok := l.AckedIndex(id); ok {
				srt[i] = uint64(idx)
				i--
			}
		}
	}

	// Sort by index. Use a bespoke algorithm (copied from the stdlib's sort
	// package) to keep srt on the stack.
	C.cinsertionSort(unsafe.Pointer(&srt[0]), C.int(n))

	// The smallest index into the array for which the value is acked by a
	// quorum. In other words, from the end of the slice, move n/2+1 to the
	// left (accounting for zero-indexing).
	pos := n - (n/2 + 1)
	return Index(srt[pos])
}

// VoteResult takes a mapping of voters to yes/no (true/false) votes and returns
// a result indicating whether the vote is pending (i.e. neither a quorum of
// yes/no has been reached), won (a quorum of yes has been reached), or lost (a
// quorum of no has been reached).
func (c MajorityConfig) VoteResult(votes map[uint64]bool) VoteResult {
	if len(c) == 0 {
		// By convention, the elections on an empty config win. This comes in
		// handy with joint quorums because it'll make a half-populated joint
		// quorum behave like a majority quorum.
		return VoteWon
	}

	var votedCnt int //vote counts for yes.
	var missing int
	for id := range c {
		v, ok := votes[id]
		if !ok {
			missing++
			continue
		}
		if v {
			votedCnt++
		}
	}

	q := len(c)/2 + 1
	if votedCnt >= q {
		return VoteWon
	}
	if votedCnt+missing >= q {
		return VotePending
	}
	return VoteLost
}
