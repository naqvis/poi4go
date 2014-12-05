// Developer: Ali Naqvi
//
// This program or package and any associated files are licensed under the
// Apache License, Version 2.0 (the "License"); you may not use these files
// except in compliance with the License. You can get a copy of the License
// at: http://www.apache.org/licenses/LICENSE-2.0.
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package poifs

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
)

const (
	// useful offsets
	_signature_offset        = 0
	_bat_count_offset        = 0x2C
	_property_start_offset   = 0x30
	_sbat_start_offset       = 0x3C
	_sbat_block_count_offset = 0x40
	_xbat_start_offset       = 0x44
	_xbat_count_offset       = 0x48
	_bat_array_offset        = 0x4c
)

var (
	_max_bats_in_header = (SMALLER_BIG_BLOCK_SIZE - _bat_array_offset) / INT_SIZE // If 4k blocks, rest is blank
)

type HeaderBlock struct {
	bigBlockSize   POIFSBigBlockSize
	bat_count      int
	property_start int
	sbat_start     int
	sbat_count     int
	xbat_start     int
	xbat_count     int
	data           []byte
}

//Create Header Block initialized with default values
func NewHeaderBlock(bigBlockSize POIFSBigBlockSize) *HeaderBlock {
	h := new(HeaderBlock)
	h.bigBlockSize = bigBlockSize

	// Set all default values. Out data is always 512 no matter what
	h.data = make([]byte, SMALLER_BIG_BLOCK_SIZE)
	for i := 0; i < len(h.data); i++ {
		h.data[i] = byte(0xFF)
	}
	//Set all the default values
	binary.LittleEndian.PutUint64(h.data, SIGNATURE)
	NewIntegerField(0x08, 0, h.data)
	NewIntegerField(0x0c, 0, h.data)
	NewIntegerField(0x10, 0, h.data)
	NewIntegerField(0x14, 0, h.data)
	NewShortField(0x18, int16(0x3b), h.data)
	NewShortField(0x1a, int16(0x3), h.data)
	NewShortField(0x1c, int16(-2), h.data)

	NewShortField(0x1e, bigBlockSize.HeaderValue, h.data)
	NewIntegerField(0x20, 0x6, h.data)
	NewIntegerField(0x24, 0, h.data)
	NewIntegerField(0x28, 0, h.data)
	NewIntegerField(0x34, 0, h.data)
	NewIntegerField(0x38, 0x1000, h.data)

	// Initialize the variables
	h.property_start = END_OF_CHAIN
	h.sbat_start = END_OF_CHAIN
	h.xbat_start = END_OF_CHAIN

	return h
}

func newHeaderBlockFromBytes(data []byte) (*HeaderBlock, error) {
	hb := &HeaderBlock{data: data}
	signature := binary.LittleEndian.Uint64(hb.data[0:8])
	if signature != SIGNATURE {
		if hb.data[0] == OOXML_FILE_HEADER[0] && hb.data[1] == OOXML_FILE_HEADER[1] &&
			hb.data[2] == OOXML_FILE_HEADER[2] && hb.data[3] == OOXML_FILE_HEADER[2] {
			return nil, OOXML_FILE_FORMAT
		}
		if (signature & 0xFF8FFFFFFFFFFFFF) == 0x0010000200040009 {
			return nil, errors.New("The supplied data appears to be in BIFF2 format. Application only supports BIFF8 format")
			//panic("The supplied data appears to be in BIFF2 format. Application only supports BIFF8 format")
		}
		//panic("Unknown header signature")
		return nil, errors.New("Unknown header signature")
	}
	// Figure out our block size
	if hb.data[30] == 12 {
		hb.bigBlockSize = LARGER_BIG_BLOCK_SIZE_DETAILS
	} else if hb.data[30] == 9 {
		hb.bigBlockSize = SMALLER_BIG_BLOCK_SIZE_DETAILS
	} else {
		return nil, errors.New("Unsupported block size (2^ " + strconv.Itoa(int(hb.data[30])) + ").Expected 2^9 or 2^12.")
		//panic("Unsupported block size (2^ " + strconv.Itoa(int(h.SectorSize)) + ").Expected 2^9 or 2^12.")
	}

	// Setup the fields to read and write the counts and starts

	tmp, err := NewIntegerFieldFromBytes(_bat_count_offset, hb.data)
	if err != nil {
		return nil, err
	}
	hb.bat_count = tmp.value

	tmp, err = NewIntegerFieldFromBytes(_property_start_offset, hb.data)
	if err != nil {
		return nil, err
	}
	hb.property_start = tmp.value

	tmp, err = NewIntegerFieldFromBytes(_sbat_start_offset, hb.data)
	if err != nil {
		return nil, err
	}
	hb.sbat_start = tmp.value

	tmp, err = NewIntegerFieldFromBytes(_sbat_block_count_offset, hb.data)
	if err != nil {
		return nil, err
	}
	hb.sbat_count = tmp.value

	tmp, err = NewIntegerFieldFromBytes(_xbat_start_offset, hb.data)
	if err != nil {
		return nil, err
	}
	hb.xbat_start = tmp.value

	tmp, err = NewIntegerFieldFromBytes(_xbat_count_offset, hb.data)
	if err != nil {
		return nil, err
	}
	hb.xbat_count = tmp.value

	return hb, nil
}

func NewHeaderBlockFromReader(r io.Reader) (*HeaderBlock, error) {
	// Grab the first 512 bytes
	// (For 4096 sized blocks, the remaining 3584 bytes are zero)
	// Then, process the contents
	head, err := readFirst512(r)
	if err != nil {
		return nil, err
	}
	hb, err := newHeaderBlockFromBytes(head)
	if err != nil {
		return nil, err
	}
	// Fetch the rest of the block if needed
	if hb.bigBlockSize.BigBlockSize != 512 {
		rest := hb.bigBlockSize.BigBlockSize - 512
		tmp := make([]byte, rest)
		io.ReadFull(r, tmp)
	}
	return hb, nil
}

func readFirst512(r io.Reader) ([]byte, error) {
	data := make([]byte, 512)
	bsCount, err := io.ReadAtLeast(r, data, 512)
	if bsCount != 512 {
		return data, errors.New(fmt.Sprintf("Unable to read entire header. %d read; expected %d bytes",
			bsCount, 512))
	} else if err != nil {
		return nil, err
	}
	return data, nil
}

func longToEx(val uint64) string {
	tmp := make([]byte, 8)
	binary.LittleEndian.PutUint64(tmp, val)
	return hex.EncodeToString(tmp)
}

func (hb *HeaderBlock) GetPropertyStart() int {
	return hb.property_start
}

func (hb *HeaderBlock) SetPropertyStart(startBlock int) {
	hb.property_start = startBlock
}

//return start of small block (MiniFAT) allocation table
func (hb *HeaderBlock) GetSBATStart() int {
	return hb.sbat_start
}
func (hb *HeaderBlock) GetSBATCount() int {
	return hb.sbat_count
}

func (hb *HeaderBlock) SetSBATStart(startBlock int) {
	hb.sbat_start = startBlock
}

func (hb *HeaderBlock) SetSBATBlockCount(count int) {
	hb.sbat_count = count
}

func (hb *HeaderBlock) GetBATCount() int {
	return hb.bat_count
}

func (hb *HeaderBlock) SetBATCount(count int) {
	hb.bat_count = count
}

func (hb *HeaderBlock) GetBATArray() []int {
	result := make([]int, min(hb.bat_count, _max_bats_in_header))
	offset := _bat_array_offset
	for j := 0; j < len(result); j++ {
		result[j] = getInt(hb.data, offset)
		offset += INT_SIZE
	}
	return result
}

func (hb *HeaderBlock) setBATArray(bat_array []int) {
	count := min(len(bat_array), _max_bats_in_header)
	blank := _max_bats_in_header - count

	var offset int = _bat_array_offset

	for i := 0; i < count; i++ {
		putInt(hb.data, offset, bat_array[i])
		offset += INT_SIZE
	}
	for i := 0; i < blank; i++ {
		putInt(hb.data, offset, UNUSED_BLOCK)
		offset += INT_SIZE
	}
}

func (hb *HeaderBlock) GetXBATCount() int {
	return hb.xbat_count
}

func (hb *HeaderBlock) SetXBATCount(count int) {
	hb.xbat_count = count
}

func (hb *HeaderBlock) GetXBATIndex() int {
	return hb.xbat_start
}

func (hb *HeaderBlock) SetXBATStart(sb int) {
	hb.xbat_start = sb
}

func (hb *HeaderBlock) GetBigBlockSize() POIFSBigBlockSize {
	return hb.bigBlockSize
}

func (hb *HeaderBlock) WriteData(w io.Writer) error {
	// Update the counts and start positions
	NewIntegerField(_bat_count_offset, hb.bat_count, hb.data)
	NewIntegerField(_property_start_offset, hb.property_start, hb.data)
	NewIntegerField(_sbat_start_offset, hb.sbat_start, hb.data)
	NewIntegerField(_sbat_block_count_offset, hb.sbat_count, hb.data)
	NewIntegerField(_xbat_start_offset, hb.xbat_start, hb.data)
	NewIntegerField(_xbat_count_offset, hb.xbat_count, hb.data)

	// Write the main data out
	_, err := w.Write(hb.data[0:512])
	if err != nil {
		return err
	}
	// Now do the padding if needed
	tmp := make([]byte, 0)
	for i := SMALLER_BIG_BLOCK_SIZE; i < hb.bigBlockSize.BigBlockSize; i++ {
		tmp = append(tmp, byte(0))
	}
	if len(tmp) > 0 {
		_, err := w.Write(tmp)
		if err != nil {
			return err
		}
	}
	return nil
}

type HeaderBlockWriter struct {
	header_block *HeaderBlock
}

func NewHeaderBlockWriter(bigBlockSize POIFSBigBlockSize) *HeaderBlockWriter {
	return &HeaderBlockWriter{header_block: NewHeaderBlock(bigBlockSize)}
}

func NewHeaderBlockWriterFromHeaderBlock(hb *HeaderBlock) *HeaderBlockWriter {
	return &HeaderBlockWriter{header_block: hb}
}

func (w *HeaderBlockWriter) SetBATBlocks(blockCount, startBlock int) []*BATBlock {
	rval := make([]*BATBlock, 0)
	bigBlockSize := w.header_block.bigBlockSize

	w.header_block.SetBATCount(blockCount)

	//Set the BAT locations
	limit := min(blockCount, _max_bats_in_header)
	bat_blocks := make([]int, limit)
	for j := 0; j < limit; j++ {
		bat_blocks[j] = startBlock + j
	}
	w.header_block.setBATArray(bat_blocks)

	// Now do the XBATs
	if blockCount > _max_bats_in_header {
		excess_blocks := blockCount - _max_bats_in_header
		excess_block_array := make([]int, excess_blocks)
		for j := 0; j < excess_blocks; j++ {
			excess_block_array[j] = startBlock + j + _max_bats_in_header
		}
		rval = CreateXBATBlocks(bigBlockSize, excess_block_array,
			startBlock+blockCount)
		w.header_block.SetXBATStart(startBlock + blockCount)
	} else {
		rval = CreateXBATBlocks(bigBlockSize, make([]int, 0), 0)
		w.header_block.SetXBATStart(END_OF_CHAIN)
	}
	w.header_block.SetXBATCount(len(rval))
	return rval
}

func (w *HeaderBlockWriter) SetPropertyStart(startBlock int) {
	w.header_block.SetPropertyStart(startBlock)
}

func (w *HeaderBlockWriter) SetSBATStart(startBlock int) {
	w.header_block.SetSBATStart(startBlock)
}

func (w *HeaderBlockWriter) SetSBATBlockCount(count int) {
	w.header_block.SetSBATBlockCount(count)
}

func calculateXBATStorageRequirementsForHB(bigBlockSize POIFSBigBlockSize, blockCount int) int {
	if blockCount > _max_bats_in_header {
		return calculateXBATStorageRequirements(bigBlockSize, blockCount-_max_bats_in_header)
	} else {
		return 0
	}
}

func (w *HeaderBlockWriter) WriteBlocks(stream io.Writer) error {
	return w.header_block.WriteData(stream)
}

func (w *HeaderBlockWriter) WriteBlock(block *bytes.Buffer) {
	w.header_block.WriteData(block)
}
