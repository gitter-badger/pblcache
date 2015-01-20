//
// Copyright (c) 2014 The pblcache Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
package spc

import (
	"github.com/lpabon/goioworkload/spc1"
	"github.com/pblcache/pblcache/cache"
	"github.com/pblcache/pblcache/tests"
	"os"
	"testing"
)

func TestNewSpcInfo(t *testing.T) {

	var cache *cache.Cache

	usedirectio := false
	blocksize := 4 * KB

	s := NewSpcInfo(cache, usedirectio, blocksize)
	tests.Assert(t, s.pblcache == cache)
	tests.Assert(t, s.blocksize == blocksize)
	tests.Assert(t, len(s.asus) == 3)
	tests.Assert(t, nil != s.asus[ASU1])
	tests.Assert(t, nil != s.asus[ASU2])
	tests.Assert(t, nil != s.asus[ASU3])
	tests.Assert(t, usedirectio == s.asus[ASU1].usedirectio)
	tests.Assert(t, usedirectio == s.asus[ASU2].usedirectio)
	tests.Assert(t, usedirectio == s.asus[ASU3].usedirectio)
}

func TestSpcOpen(t *testing.T) {

	// initialize
	var cache *cache.Cache
	usedirectio := false
	blocksize := 4 * KB
	s := NewSpcInfo(cache, usedirectio, blocksize)

	// Get a test file
	tmpfile := tests.Tempfile()

	// No file exists
	err := s.Open(1, tmpfile)
	tests.Assert(t, err != nil)

	// Create the file and open it
	buf := make([]byte, 16*4*KB)
	fp, err := os.Create(tmpfile)
	tests.Assert(t, err == nil)
	fp.Write(buf)
	fp.Close()
	defer os.Remove(tmpfile)

	// Now open, and it should work
	err = s.Open(1, tmpfile)
	tests.Assert(t, err == nil)
}

func TestSpcAdjustAsuSizes(t *testing.T) {

	// initialize
	var cache *cache.Cache
	usedirectio := false
	blocksize := 4 * KB
	s := NewSpcInfo(cache, usedirectio, blocksize)

	// Setup some fake data
	s.asus[ASU1].len = 100
	s.asus[ASU2].len = 200
	s.asus[ASU3].len = 50
	err := s.adjustAsuSizes()

	// asu1 must be equal to asu2
	tests.Assert(t, err == nil)
	tests.Assert(t, s.asus[ASU1].len == 100)
	tests.Assert(t, s.asus[ASU2].len == 100)
	tests.Assert(t, s.asus[ASU3].len == 22)

	// Setup some fake data
	s.asus[ASU1].len = 200
	s.asus[ASU2].len = 100
	s.asus[ASU3].len = 50
	err = s.adjustAsuSizes()

	// asu1 must be equal to asu2
	tests.Assert(t, err == nil)
	tests.Assert(t, s.asus[ASU1].len == 100)
	tests.Assert(t, s.asus[ASU2].len == 100)
	tests.Assert(t, s.asus[ASU3].len == 22)

	// Setup some fake data
	s.asus[ASU1].len = 100
	s.asus[ASU2].len = 100
	s.asus[ASU3].len = 5
	err = s.adjustAsuSizes()

	// asu3 will error since it is not large enough
	tests.Assert(t, err != nil)
	tests.Assert(t, s.asus[ASU1].len == 100)
	tests.Assert(t, s.asus[ASU2].len == 100)
	tests.Assert(t, s.asus[ASU3].len == 5)

}

// Returns size in GB
func TestSpcSize(t *testing.T) {

	// initialize
	var cache *cache.Cache
	usedirectio := false
	blocksize := 4 * KB
	s := NewSpcInfo(cache, usedirectio, blocksize)

	// Set fake len
	s.asus[ASU1].len = 40 * GB / (4 * KB)

	// Check size
	size := s.Size(1)
	tests.Assert(t, size == 40)

}

func TestSpc1Init(t *testing.T) {

	// initialize
	var cache *cache.Cache
	usedirectio := false
	blocksize := 4 * KB
	s := NewSpcInfo(cache, usedirectio, blocksize)

	// Set fake len
	s.asus[ASU1].len = 55
	s.asus[ASU2].len = 45

	// Set asu3 to a value that is too small
	s.asus[ASU3].len = 1

	bsu := 50
	contexts := 1
	err := s.Spc1Init(bsu, contexts)

	// It should not accept a value of asu3
	// because it is too small
	tests.Assert(t, err != nil)

	// Set fake len
	s.asus[ASU1].len = 55
	s.asus[ASU2].len = 45
	s.asus[ASU3].len = 10
	err = s.Spc1Init(bsu, contexts)

	// Now it should succeed
	tests.Assert(t, err == nil)

	// Check that the sizes where adjusted
	tests.Assert(t, s.asus[ASU1].len == 45)
	tests.Assert(t, s.asus[ASU2].len == 45)
	tests.Assert(t, s.asus[ASU3].len == 10)

	// Check spc1 was initialized
	io := spc1.NewSpc1Io(1)
	err = io.Generate()
	tests.Assert(t, err == nil)
}
