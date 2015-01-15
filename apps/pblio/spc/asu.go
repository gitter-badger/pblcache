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
	"github.com/lpabon/godbc"
	"os"
	"syscall"
)

const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
)

type Asu struct {
	fps         *os.File
	len         uint32
	usedirectio bool
}

func NewAsu(usedirectio bool) *Asu {
	return &Asu{
		usedirectio: usedirectio,
	}
}

// Size in GB
func (a *Asu) Size() float64 {
	return float64(a.len) * 4 * KB / GB
}

func (a *Asu) Open(filename string) error {
	var err error

	godbc.Require(filename != "")

	// Set the appropriate flags
	flags := os.O_RDWR | os.O_EXCL
	if a.usedirectio {
		flags |= syscall.O_DIRECT
	}

	// Open the file
	a.fps, err = os.OpenFile(filename, flags, os.ModePerm)
	if err != nil {
		return err
	}

	// Get storage size
	var size int64
	size, err = a.fps.Seek(0, os.SEEK_END)
	if err != nil {
		return err
	}
	a.len = uint32(size / int64(4*KB))

	godbc.Ensure(a.fps != nil, a.fps)
	godbc.Ensure(a.len > 0, a.len)

	return nil
}
