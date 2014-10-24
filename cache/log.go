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

package cache

import (
	"fmt"
	"github.com/lpabon/buffercache"
	"github.com/lpabon/bufferio"
	"github.com/lpabon/godbc"
	"github.com/pblcache/pblcache/message"
	"os"
	"sync"
	"syscall"
	"time"
)

const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
	TB = 1024 * GB

	fdirectio       = false
	fsegmentbuffers = 32
	fsegmentsize    = 1024
)

/*
func init() {
	// These values are set by the main program when it calls flag.Parse()
	flag.BoolVar(&fdirectio, "iodb_directio", false, "\n\tUse DIRECTIO in iodb")
	flag.IntVar(&fsegmentbuffers, "iodb_segmentbuffers", 32, "\n\tNumber of inflight buffers")
	flag.IntVar(&fsegmentsize, "iodb_segmentsize", 1024, "\n\tSegment size in KB")
}
*/

type IoSegment struct {
	segmentbuf []byte
	data       *bufferio.BufferIO
	offset     uint64
	written    bool
	lock       sync.RWMutex
}

type Log struct {
	size           uint64
	blocksize      uint64
	segmentsize    uint64
	numsegments    uint64
	blocks         uint64
	segments       []IoSegment
	segment        *IoSegment
	segmentbuffers int
	chwriting      chan *IoSegment
	chreader       chan *IoSegment
	chavailable    chan *IoSegment
	wg             sync.WaitGroup
	current        uint64
	maxentries     uint64
	fp             *os.File
	wrapped        bool
	stats          *logstats
	bc             buffercache.BufferCache
	Msgchan        chan *message.Message
	quitchan       chan struct{}
	logreaders     chan *message.Message
}

func NewLog(logfile string, blocks, blocksize, blocks_per_segment, bcsize uint64) (*Log, uint64) {

	var err error

	log := &Log{}
	log.stats = &logstats{}
	log.blocksize = blocksize
	log.segmentsize = blocks_per_segment * blocksize
	log.maxentries = log.segmentsize / log.blocksize

	// We have to make sure that the number of blocks requested
	// fit into the segments tracked by the log
	log.numsegments = blocks / log.maxentries
	log.blocks = log.numsegments * log.maxentries
	log.size = log.numsegments * log.segmentsize

	if log.numsegments < fsegmentbuffers {
		log.segmentbuffers = int(log.numsegments)
	} else {
		log.segmentbuffers = fsegmentbuffers
	}
	godbc.Check(log.numsegments != 0,
		fmt.Sprintf("bs:%v ssize:%v sbuffers:%v blocks:%v max:%v ns:%v size:%v\n",
			log.blocksize, log.segmentsize, log.segmentbuffers, log.blocks,
			log.maxentries, log.numsegments, log.size))

	// Create buffer cache
	log.bc = buffercache.NewClockCache(bcsize, log.blocksize)

	// Incoming message channel
	log.Msgchan = make(chan *message.Message, 32)
	log.quitchan = make(chan struct{})
	log.logreaders = make(chan *message.Message, 32)

	// Segment channel state machine:
	// 		-> Client writes available segment
	// 		-> Segment written to storage
	// 		-> Segment read from storage
	// 		-> Segment available
	log.chwriting = make(chan *IoSegment, log.segmentbuffers)
	log.chavailable = make(chan *IoSegment, log.segmentbuffers)
	log.chreader = make(chan *IoSegment, log.segmentbuffers)

	// Set up each of the segments
	log.segments = make([]IoSegment, log.segmentbuffers)
	for i := 0; i < log.segmentbuffers; i++ {
		log.segments[i].segmentbuf = make([]byte, log.segmentsize)
		log.segments[i].data = bufferio.NewBufferIO(log.segments[i].segmentbuf)

		// Fill ch available with all the available buffers
		log.chreader <- &log.segments[i]
	}

	// Set up the first available segment
	log.segment = <-log.chreader

	// Open the storage device
	os.Remove(logfile)

	// For DirectIO
	if fdirectio {
		log.fp, err = os.OpenFile(logfile, syscall.O_DIRECT|os.O_CREATE|os.O_RDWR|os.O_EXCL, os.ModePerm)
	} else {
		log.fp, err = os.OpenFile(logfile, os.O_CREATE|os.O_RDWR|os.O_EXCL, os.ModePerm)
	}
	godbc.Check(err == nil)

	//err = syscall.Fallocate(int(log.fp.Fd()), 0, 0, int64(blocks*blocksize))
	//godbc.Check(err == nil)

	godbc.Ensure(log.size != 0)
	godbc.Ensure(log.blocksize == blocksize)
	godbc.Ensure(log.Msgchan != nil)
	godbc.Ensure(log.chwriting != nil)
	godbc.Ensure(log.chavailable != nil)
	godbc.Ensure(log.chreader != nil)
	godbc.Ensure(log.segmentbuffers == len(log.segments))
	godbc.Ensure(log.segmentbuffers-1 == len(log.chreader))
	godbc.Ensure(0 == len(log.chavailable))
	godbc.Ensure(0 == len(log.chwriting))
	godbc.Ensure(nil != log.segment)

	// Now that we are sure everything is clean,
	// we can start the goroutines
	for i := 0; i < 32; i++ {
		log.wg.Add(1)
		go log.logread()
	}
	log.server()
	log.writer()
	log.reader()

	// Return the log object to the caller.
	// Also return the maximum number of blocks, which may
	// be different from what the caller asked.  The log
	// will make sure that the maximum number of blocks
	// are contained per segment
	return log, log.blocks
}

func (c *Log) logread() {
	defer c.wg.Done()
	for m := range c.logreaders {
		iopkt := m.IoPkt()
		offset := c.offset(iopkt.BlockNum)

		// Read from storage
		start := time.Now()
		n, err := c.fp.ReadAt(iopkt.Buffer, int64(offset))
		end := time.Now()
		c.stats.ReadTimeRecord(end.Sub(start))

		godbc.Check(uint64(n) == c.blocksize,
			fmt.Sprintf("Read %v expected %v from location %v index %v",
				n, c.blocksize, offset, iopkt.BlockNum))
		godbc.Check(err == nil)
		c.stats.StorageHit()

		// Save in buffer cache
		c.bc.Set(offset, iopkt.Buffer)

		// Return to caller
		m.Done()
	}
}

func (c *Log) server() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		emptychan := false
		for {
			// Check if we have been signaled through <-quit
			// If we have, we now know that as soon as the
			// message channel is empty, we can quit.
			if emptychan {
				if len(c.Msgchan) == 0 {
					break
				}
			}

			select {
			case msg := <-c.Msgchan:
				switch msg.Type {
				case message.MsgPut:
					c.put(msg)
				case message.MsgGet:
					c.get(msg)
				}
			case <-c.quitchan:
				// :TODO: Ok for now, but we cannot just quit
				// We need to empty the Iochan
				emptychan = true
			}
		}

		// We are closing the log.  Need to shut down the channels
		if c.segment.written {
			c.sync()
		}
		close(c.chwriting)
		close(c.logreaders)

	}()
}

func (c *Log) writer() {

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for s := range c.chwriting {
			if s.written {
				start := time.Now()
				n, err := c.fp.WriteAt(s.segmentbuf, int64(s.offset))
				end := time.Now()
				s.written = false

				c.stats.WriteTimeRecord(end.Sub(start))
				godbc.Check(n == len(s.segmentbuf))
				godbc.Check(err == nil)
			} else {
				c.stats.SegmentSkipped()
			}
			c.chreader <- s
		}
		close(c.chreader)
	}()

}

func (c *Log) reader() {

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for s := range c.chreader {
			s.lock.Lock()

			// Reset the bufferIO managers
			s.data.Reset()

			// Move to the next offset
			c.current += c.segmentsize
			c.current = c.current % c.size

			if 0 == c.current {
				c.stats.Wrapped()
				c.wrapped = true
			}
			s.offset = c.current

			if c.wrapped {
				start := time.Now()
				n, err := c.fp.ReadAt(s.segmentbuf, int64(s.offset))
				end := time.Now()
				c.stats.SegmentReadTimeRecord(end.Sub(start))
				godbc.Check(n == len(s.segmentbuf))
				godbc.Check(err == nil)
			}

			s.lock.Unlock()

			c.chavailable <- s
		}
	}()

}

func (c *Log) sync() {
	// Send to writer
	c.chwriting <- c.segment

	// Get a new available buffer
	c.segment = <-c.chavailable
}

func (c *Log) offset(index uint64) uint64 {
	return (index * c.blocksize)
}

func (c *Log) inRange(index uint64, s *IoSegment) bool {
	offset := c.offset(index)

	return ((offset >= s.offset) &&
		(offset < (s.offset + c.segmentsize)))
}

func (c *Log) put(msg *message.Message) error {

	iopkt := msg.IoPkt()
	godbc.Require(iopkt.BlockNum < c.blocks)

	// Make sure the block number curresponds to the
	// current segment.  If not, c.sync() will place
	// the next available segment into c.segment
	for !c.inRange(iopkt.BlockNum, c.segment) {
		c.sync()
	}

	// get log offset
	offset := c.offset(iopkt.BlockNum)

	// Buffer cache is a Read-miss cache
	c.bc.Invalidate(iopkt.BlockNum)

	// Write to current buffer
	n, err := c.segment.data.WriteAt(iopkt.Buffer, int64(offset-c.segment.offset))
	godbc.Check(n == len(iopkt.Buffer))
	godbc.Check(err == nil)

	c.segment.written = true

	// We have written the data, and we are done with the message
	msg.Done()

	return err
}

func (c *Log) get(msg *message.Message) error {

	var n int
	var err error

	iopkt := msg.IoPkt()
	offset := c.offset(iopkt.BlockNum)

	err = c.bc.Get(iopkt.BlockNum, iopkt.Buffer)
	if err == nil {
		c.stats.BufferHit()
		msg.Done()
		return nil
	}

	// Check if the data is in RAM.  Go through each buffered segment
	for i := 0; i < c.segmentbuffers; i++ {

		c.segments[i].lock.RLock()

		if c.inRange(iopkt.BlockNum, &c.segments[i]) {

			n, err = c.segments[i].data.ReadAt(iopkt.Buffer, int64(offset-c.segments[i].offset))

			godbc.Check(err == nil)
			godbc.Check(uint64(n) == c.blocksize,
				fmt.Sprintf("Read %v expected:%v from location:%v iopkt.BlockNum:%v",
					n, c.blocksize, offset, iopkt.BlockNum))
			c.stats.RamHit()

			c.segments[i].lock.RUnlock()

			// Save in buffer cache
			c.bc.Set(iopkt.BlockNum, iopkt.Buffer)

			// Return message
			msg.Done()

			return nil
		}

		c.segments[i].lock.RUnlock()
	}

	// We do not have the data yet, so we need to
	// read it from the storage system
	c.logreaders <- msg

	return nil
}

func (c *Log) Close() {

	// Shut down server first
	close(c.quitchan)
	c.wg.Wait()

	// Close the storage
	c.fp.Close()
}

func (c *Log) String() string {
	return fmt.Sprintf(
		"== Log Information ==\n") +
		c.stats.Stats().String()
}

func (c *Log) Stats() *LogStats {
	return c.stats.Stats()
}
