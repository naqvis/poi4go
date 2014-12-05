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
	"errors"
	"fmt"
	"io"
	"log"
	"math"
)

const (
	BLOCK_SHIFT     = 6
	_block_size     = 1 << BLOCK_SHIFT
	BLOCK_MASK      = _block_size - 1
	MAX_BLOCK_COUNT = 65535
)

type BlockWritable interface {
	WriteBlocks(w io.Writer) error
}

type ListManagedBlock interface {
	Data() ([]byte, error)
}

type BlockList interface {
	Zap(index int)
	Remove(index int) (ListManagedBlock, error)
	Fetch(startBlock, headerPropertiesStartBlock int) ([]ListManagedBlock, error)
	SetBAT(bat *BlockAllocationTableReader) error
	BlockCount() int
}

type rawDataBlock struct {
	data         []byte
	eof, hasdata bool
}

type BlockAllocationTableReader struct {
	enteries     []int
	bigBlockSize POIFSBigBlockSize
}

type smallDocumentBlock struct {
	data                 []byte
	blocks_per_big_block int
	bigBlockSize         POIFSBigBlockSize
}

type smallDocumentBlockList struct {
	*rawDataBlockList
}

type DocumentBlock struct {
	bigBlockSize POIFSBigBlockSize
	data         []byte
	bytes_read   int
}

type DataInputBlock struct {
	buf                 []byte
	readIndex, maxIndex int
}

type PropertyBlock struct {
	bigBlockSize POIFSBigBlockSize
	properties   []IProperty
}

type BATBlock struct {
	bigBlockSize     POIFSBigBlockSize
	values           []int
	has_free_sectors bool
	ourBlockIndex    int
}

type BATBlockAndIndex struct {
	index int
	block *BATBlock
}

type rawDataBlockList struct {
	blocks []ListManagedBlock
	bat    *BlockAllocationTableReader
}

func RawDataBlockList(r io.Reader, bigBlockSize POIFSBigBlockSize) (*rawDataBlockList, error) {
	raw := new(rawDataBlockList)
	raw.blocks = make([]ListManagedBlock, 0)
	for {
		block := RawDataBlock(r, bigBlockSize.BigBlockSize)
		if block.HasData() {
			raw.blocks = append(raw.blocks, block)
		}
		if block.EOF() {
			break
		}
	}
	return raw, nil
}

func (raw *rawDataBlockList) Zap(index int) {
	if index >= 0 && index < len(raw.blocks) {
		raw.blocks[index] = nil
	}
}

func (raw *rawDataBlockList) Remove(index int) (ListManagedBlock, error) {
	if index < 0 || index >= len(raw.blocks) {
		return nil, errors.New("Invalid index to remove")
	}
	res := raw.blocks[index]
	if res == nil {
		return nil, errors.New("block already removed. Does your POIFS has circular references?")
	}
	raw.blocks[index] = nil
	return res, nil
}

func (raw *rawDataBlockList) Fetch(startBlock, headerPropertiesStartBlock int) ([]ListManagedBlock, error) {
	if raw.bat == nil {
		return nil, errors.New("Improperly initialized list: no block allocation table provided")
	}
	return raw.bat.fetchBlocks(startBlock, headerPropertiesStartBlock, raw)
}

func (raw *rawDataBlockList) SetBAT(bat *BlockAllocationTableReader) error {
	if raw.bat != nil {
		return errors.New("Attemp to replace existing allocation table")
	}
	raw.bat = bat
	return nil
}

func (raw *rawDataBlockList) BlockCount() int {
	return len(raw.blocks)
}

func RawDataBlock(r io.Reader, blocksize int) *rawDataBlock {
	rdb := new(rawDataBlock)
	rdb.data = make([]byte, blocksize)
	count, err := io.ReadFull(r, rdb.data)
	rdb.hasdata = err == nil && count > 0
	rdb.eof = false
	if err != nil {
		if err == io.EOF {
			rdb.eof = true
		} else {
			rdb.eof = true
			log.Printf("Unable to read entire block; %d byte read before EOF;"+
				"Expected %d bytes. Your document might have been corrupted or truncated",
				count, blocksize)
		}
	}

	return rdb
}

func (raw *rawDataBlock) Data() ([]byte, error) {
	if !raw.hasdata {
		return nil, EMPTY_BLOCK_ERROR
	}
	return raw.data, nil
}

func (raw *rawDataBlock) EOF() bool {
	return raw.eof
}

func (raw *rawDataBlock) HasData() bool {
	return raw.hasdata
}

func (raw *rawDataBlock) BigBlockSize() int {
	return len(raw.data)
}

func NewBlockAllocationTableReader1(bigBlockSize POIFSBigBlockSize,
	blocks []ListManagedBlock, raw_blocklist BlockList) (*BlockAllocationTableReader, error) {

	batr := new(BlockAllocationTableReader)
	batr.bigBlockSize = bigBlockSize
	batr.enteries = make([]int, 0)
	batr.setEntries(blocks, raw_blocklist)
	return batr, nil
}

func NewBlockAllocationTableReader(bigBlockSize POIFSBigBlockSize, block_count int, block_array []int,
	xbat_count, xbat_index int, raw_blocklist BlockList) (*BlockAllocationTableReader, error) {
	if block_count < 0 || block_count > MAX_BLOCK_COUNT {
		return nil, errors.New("Illegal block count. Block count should be > 0 and less than 65535")
	}

	limit := min(block_count, len(block_array))
	blocks := make([]ListManagedBlock, block_count)
	var block_index int
	for block_index = 0; block_index < limit; block_index++ {
		next_offset := int(block_array[block_index])
		if next_offset > raw_blocklist.BlockCount() {
			msg := fmt.Sprintf("Your file contains %d  sectors, "+
				"but the initial DIFAT array at index %d"+
				" referenced block # %d . This isn't allowed and "+
				" your file is corrupt", raw_blocklist.BlockCount()+1, block_index, next_offset)
			return nil, errors.New(msg)
		}
		block, err := raw_blocklist.Remove(next_offset)
		if err != nil {
			return nil, err
		}
		blocks[block_index] = block
		//blocks = append(blocks, block) //.(*rawDataBlock))
	}

	if block_index < block_count {
		if xbat_index < 0 {
			return nil, errors.New("BAT count exceeds limit, yet XBAT index indicates no valid entries")
		}

		chain_index := xbat_index
		max_entries_per_block := bigBlockSize.XBATEntriesPerBlock()
		chain_index_offset := bigBlockSize.NextXBATChainOffset()
		for j := 0; j < xbat_count; j++ {
			limit = min(block_count-block_index, max_entries_per_block)
			block, err := raw_blocklist.Remove(chain_index)
			if err != nil {
				return nil, err
			}
			data, err := block.Data()
			if err != nil {
				return nil, err
			}
			offset := 0
			for k := 0; k < limit; k++ {
				block, err := raw_blocklist.Remove(getInt(data, offset))
				if err != nil {
					return nil, err
				}
				blocks[block_index] = block //.(*rawDataBlock)
				block_index += 1
				offset += INT_SIZE
			}
			chain_index = getInt(data, chain_index_offset)
			if chain_index == END_OF_CHAIN {
				break
			}
		}
	}
	if block_index != block_count {
		return nil, errors.New("Could not find all blocks")
	}
	// Now that we have all of the raw data blocks which make
	//  up the FAT, go through and create the indices
	batr := new(BlockAllocationTableReader)
	batr.bigBlockSize = bigBlockSize
	batr.enteries = make([]int, 0)
	batr.setEntries(blocks, raw_blocklist)
	return batr, nil
}

func (batr *BlockAllocationTableReader) setEntries(blocks []ListManagedBlock, raw_blocks BlockList) {
	limit := batr.bigBlockSize.BATEntriesPerBlock()

	for block_index, block := range blocks {
		data, err := block.Data()
		if err != nil {
			panic(err)
		}

		offset := 0
		for k := 0; k < limit; k++ {
			entry := getInt(data, offset)
			if entry == UNUSED_BLOCK {
				raw_blocks.Zap(len(batr.enteries))
			}

			batr.enteries = append(batr.enteries, entry)
			offset += INT_SIZE
		}
		blocks[block_index] = nil
	}
	raw_blocks.SetBAT(batr)
}

func (batr *BlockAllocationTableReader) fetchBlocks(startBlock, headerPropertiesStartBlock int,
	blockList BlockList) ([]ListManagedBlock, error) {

	res := make([]ListManagedBlock, 0)
	currentBlock := startBlock
	firstPass := true

	for currentBlock != END_OF_CHAIN {
		// Grab the data at the current block offset
		dataBlock, err := blockList.Remove(currentBlock)
		if err != nil {
			if currentBlock == headerPropertiesStartBlock {
				log.Println("Warning, header block comes after data blocks in POIFS block list")
				currentBlock = END_OF_CHAIN
			} else if currentBlock == 0 && firstPass {
				log.Println("Warning, incorrectly terminated empty data blocks in POIFS block listing (should end at -2, ended at 0)")
				currentBlock = END_OF_CHAIN
			} else {
				fmt.Printf("block: %v\n", currentBlock)
				return nil, err
			}
		}
		res = append(res, dataBlock)
		// Now figure out which block we go to next
		currentBlock = batr.enteries[currentBlock]
	}
	return res, nil
}

func NewSmallDocumentBlock(bigBlockSize POIFSBigBlockSize) *smallDocumentBlock {
	return &smallDocumentBlock{
		bigBlockSize:         bigBlockSize,
		blocks_per_big_block: bigBlockSize.BigBlockSize / _block_size,
		data:                 make([]byte, _block_size),
	}
}

func SmallDocumentBlock(bigBlockSize POIFSBigBlockSize, data []byte, index int) *smallDocumentBlock {
	sd := &smallDocumentBlock{}
	sd.bigBlockSize = bigBlockSize
	sd.blocks_per_big_block = bigBlockSize.BigBlockSize / _block_size
	sd.data = make([]byte, _block_size)
	copy(sd.data[0:], data[index*_block_size:])
	return sd
}

func SmallDocBlockFromRaw(bigBlockSize POIFSBigBlockSize, blocks []ListManagedBlock) []*smallDocumentBlock {
	blocks_per_big_block := bigBlockSize.BigBlockSize / _block_size
	res := make([]*smallDocumentBlock, 0)
	for _, block := range blocks {
		data, err := block.Data()
		if err != nil {
			continue
		}
		for k := 0; k < blocks_per_big_block; k++ {
			sd := SmallDocumentBlock(bigBlockSize, data, k)
			res = append(res, sd)
		}
	}
	return res
}

func SmallDocBlockFromDataBlock(bigBlockSize POIFSBigBlockSize, store []BlockWritable, size int) []*smallDocumentBlock {
	count := (size + _block_size - 1) / _block_size

	var buff bytes.Buffer
	for _, db := range store {
		err := db.WriteBlocks(&buff)
		if err != nil {
			panic(err)
		}
	}

	data := buff.Bytes()
	rval := make([]*smallDocumentBlock, count)
	for i := 0; i < count; i++ {
		rval[i] = SmallDocumentBlock(bigBlockSize, data, i)
	}

	return rval
}

func GetDataInputBlockFromSmallBlocks(blocks []*smallDocumentBlock, offset int) *DataInputBlock {
	var firstBlockIndex int = offset >> BLOCK_SHIFT
	var firstBlockOffset int = offset & BLOCK_MASK

	return NewDataInputBlock(blocks[firstBlockIndex].data, firstBlockOffset)
}

func (sd *smallDocumentBlock) Data() ([]byte, error) {
	if len(sd.data) <= 0 {
		return nil, EMPTY_BLOCK_ERROR
	}
	return sd.data, nil
}

func (sd *smallDocumentBlock) WriteBlocks(w io.Writer) error {
	_, err := w.Write(sd.data)
	return err
}

func SmallDocumentBlockList(blocks []*smallDocumentBlock) (*smallDocumentBlockList, error) {
	l := new(smallDocumentBlockList)
	l.rawDataBlockList = new(rawDataBlockList)
	l.blocks = make([]ListManagedBlock, 0)

	for _, block := range blocks {
		l.blocks = append(l.blocks, block)
	}

	return l, nil
}

func GetSmallDocumentBlocks(bigBlockSize POIFSBigBlockSize, blockList *rawDataBlockList, root *RootProperty,
	sbatStart int) (BlockList, error) {

	smallBlockBlocks, err := blockList.Fetch(root.start_block.value, -1)

	if err != nil {
		return nil, err
	}
	list, err := SmallDocumentBlockList(SmallDocBlockFromRaw(bigBlockSize, smallBlockBlocks))
	if err != nil {
		return nil, err
	}
	tmp, err := blockList.Fetch(sbatStart, -1)
	if err != nil {
		return nil, err
	}
	NewBlockAllocationTableReader1(bigBlockSize, tmp, list)
	return list, nil
}

func SmallDocumentBlockFill(bigBlockSize POIFSBigBlockSize, blocks *[]BlockWritable) int {
	block_per_big_block := bigBlockSize.BigBlockSize / _block_size

	count := len(*blocks)
	big_block_count := (count + block_per_big_block - 1) / block_per_big_block
	full_count := big_block_count * block_per_big_block
	for count < full_count {
		*blocks = append(*blocks, makeEmptySmallDocumentBlock(bigBlockSize))
		count += 1
	}
	return big_block_count
}

func makeEmptySmallDocumentBlock(bigBlockSize POIFSBigBlockSize) *smallDocumentBlock {
	b := NewSmallDocumentBlock(bigBlockSize)
	for i := 0; i < len(b.data); i++ {
		b.data[i] = byte(0xff)
	}

	return b
}

func DocumentBlockFromRawBlock(block *rawDataBlock) (*DocumentBlock, error) {
	db := new(DocumentBlock)
	if block.BigBlockSize() == SMALLER_BIG_BLOCK_SIZE {
		db.bigBlockSize = SMALLER_BIG_BLOCK_SIZE_DETAILS
	} else {
		db.bigBlockSize = LARGER_BIG_BLOCK_SIZE_DETAILS
	}
	var err error
	db.data, err = block.Data()
	if err != nil {
		return nil, err
	}
	db.bytes_read = len(db.data)

	return db, nil
}

func createDocumentBlock(bigBlockSize POIFSBigBlockSize) *DocumentBlock {
	db := new(DocumentBlock)
	db.bigBlockSize = bigBlockSize
	db.data = make([]byte, bigBlockSize.BigBlockSize)
	for idx, _ := range db.data {
		db.data[idx] = byte(0xFF)
	}
	return db
}

//func DocumentBlockFromStream(data []byte, bigBlockSize POIFSBigBlockSize) (*DocumentBlock, error) {
func DocumentBlockFromStream(r io.Reader, bigBlockSize POIFSBigBlockSize) (db *DocumentBlock, err error) {
	db = createDocumentBlock(bigBlockSize)
	//copy(db.data, data)
	db.bytes_read, err = io.ReadFull(r, db.data)
	//db.bytes_read = len(db.data)
	if err != nil && err != io.EOF {
		return
	}
	return db, nil
}

func convertToDocumentBlock(bigBlockSize POIFSBigBlockSize, array []byte, size int) []*DocumentBlock {
	rval := make([]*DocumentBlock,
		(size+bigBlockSize.BigBlockSize-1)/bigBlockSize.BigBlockSize)

	offset := 0
	for k := 0; k < len(rval); k++ {
		rval[k] = createDocumentBlock(bigBlockSize)
		if offset < len(array) {
			length := min(bigBlockSize.BigBlockSize, len(array)-offset)
			copy(rval[k].data[0:length], array[offset:])
			if length != bigBlockSize.BigBlockSize {
				for idx := length; idx < bigBlockSize.BigBlockSize; idx++ {
					rval[k].data[idx] = byte(0xFF)
				}
			}
		} else {
			for idx, _ := range rval[k].data {
				rval[k].data[idx] = byte(0xFF)
			}
		}
	}
	return rval
}

func GetDataInputBlockFromDocumentBlocks(blocks []*DocumentBlock, offset int) *DataInputBlock {
	if blocks == nil || len(blocks) == 0 {
		return nil
	}

	//Key things about the size of the block
	bigBlockSize := blocks[0].bigBlockSize
	block_shift := uint16(bigBlockSize.HeaderValue)
	block_size := bigBlockSize.BigBlockSize
	block_mask := block_size - 1

	//Now do the offset lookup
	var firstBlockIndex int = offset >> block_shift
	var firstBlockOffset int = offset & block_mask

	return NewDataInputBlock(blocks[firstBlockIndex].data, firstBlockOffset)
}

func (db *DocumentBlock) Size() int {
	return db.bytes_read
}

func (db *DocumentBlock) PartiallyRead() bool {
	return db.bytes_read != db.bigBlockSize.BigBlockSize
}

func (db *DocumentBlock) WriteData(w io.Writer) error { //(stream bytes.Buffer) {
	//stream.Write(db.data)
	_, err := w.Write(db.data)
	return err
}

func (db *DocumentBlock) WriteBlocks(w io.Writer) error { //WriteBlocks(stream bytes.Buffer) {
	//db.WriteData(stream)
	return db.WriteData(w)
}

func NewDataInputBlock(data []byte, startOffset int) *DataInputBlock {
	dib := new(DataInputBlock)
	dib.buf = make([]byte, len(data))
	copy(dib.buf, data)
	dib.readIndex = startOffset
	dib.maxIndex = len(data)

	return dib
}

func (d *DataInputBlock) Available() int {
	return d.maxIndex - d.readIndex
}

func (d *DataInputBlock) ReadUByte() int {
	return int(d.buf[d.readIndex] & 0xFF)
}

func (d *DataInputBlock) ReadUShortLE() int {
	i := d.readIndex
	b0 := d.buf[i] & 0xFF
	i += 1
	b1 := d.buf[i] & 0xFF
	i += 1
	d.readIndex = i
	return int((b1 << 8) + (b0 << 0))
}

// Reads a short which spans the end of prevBlock
func (d *DataInputBlock) ReadUShortLE2(prevBlock *DataInputBlock) int {
	//simple case - will always be one byte in each block
	i := len(prevBlock.buf) - 1
	b0 := prevBlock.buf[i] & 0xFF
	i += 1
	b1 := d.buf[d.readIndex] & 0xFF
	d.readIndex += 1
	return int((b1 << 8) + (b0 << 0))
}

func (d *DataInputBlock) ReadIntLE() int {
	i := d.readIndex
	b0 := int32(d.buf[i] & 0xFF)
	i += 1
	b1 := int32(d.buf[i] & 0xFF)
	i += 1
	b2 := int32(d.buf[i] & 0xFF)
	i += 1
	b3 := int32(d.buf[i] & 0xFF)
	d.readIndex = i
	return int((b3 << 24) + (b2 << 16) + (b1 << 8) + (b0 << 0))
}

func (d *DataInputBlock) ReadIntLE2(prevBlock *DataInputBlock, prevBlockAvailable int) int {
	buf := make([]byte, 4)
	d.readSpanning(prevBlock, prevBlockAvailable, buf)
	b0 := int32(buf[0] & 0xFF)
	b1 := int32(buf[1] & 0xFF)
	b2 := int32(buf[2] & 0xFF)
	b3 := int32(buf[3] & 0xFF)

	return int((b3 << 24) + (b2 << 16) + (b1 << 8) + (b0 << 0))
}

func (d *DataInputBlock) readSpanning(prevBlock *DataInputBlock, prevBlockAvailable int, buf []byte) {
	copy(buf[0:prevBlockAvailable], prevBlock.buf[prevBlock.readIndex:])
	secondRead := len(buf) - prevBlockAvailable
	copy(buf[prevBlockAvailable:], d.buf[0:])
	d.readIndex = secondRead
}

func (d *DataInputBlock) ReadFully(buf []byte, offset, length int) {
	copy(buf[offset:], d.buf[d.readIndex:d.readIndex+length])
	d.readIndex += length
}

func NewPropertyBlock(bigBlockSize POIFSBigBlockSize) *PropertyBlock {
	pb := new(PropertyBlock)
	pb.bigBlockSize = bigBlockSize
	return pb
}

func createPropBlock(bigBlockSize POIFSBigBlockSize, props []IProperty, offset int) *PropertyBlock {
	pb := NewPropertyBlock(bigBlockSize)
	pb.properties = make([]IProperty, bigBlockSize.PropertiesPerBlock())
	for j := 0; j < len(pb.properties); j++ {
		pb.properties[j] = props[j+offset]
	}

	return pb
}

/*
* Create an array of PropertyBlocks from an array of Property
* instances, creating empty Property instances to make up any
* shortfall
 */
func NewPropertyBlockArray(bigBlockSize POIFSBigBlockSize, props []IProperty) []BlockWritable {
	properties_per_block := bigBlockSize.PropertiesPerBlock()
	block_count := (len(props) + properties_per_block - 1) / properties_per_block
	to_be_written := make([]IProperty, block_count*properties_per_block)

	copy(to_be_written[0:], props[0:])
	for j := len(props); j < len(to_be_written); j++ {
		to_be_written[j] = newBlockProperty()
	}

	rval := make([]BlockWritable, block_count)
	for j := 0; j < block_count; j++ {
		rval[j] = createPropBlock(bigBlockSize, to_be_written, j*properties_per_block)
	}

	return rval
}

func (pb *PropertyBlock) WriteBlocks(w io.Writer) error {
	for j := 0; j < pb.bigBlockSize.PropertiesPerBlock(); j++ {
		err := pb.properties[j].WriteData(w)
		if err != nil {
			return err
		}
	}
	return nil
}

//Create a single instance initialized with default values
func NewBATBlock(bigBlockSize POIFSBigBlockSize) *BATBlock {
	bb := &BATBlock{bigBlockSize: bigBlockSize,
		values:           make([]int, bigBlockSize.BATEntriesPerBlock()),
		has_free_sectors: true}
	for i := 0; i < len(bb.values); i++ {
		bb.values[i] = UNUSED_BLOCK
	}
	return bb
}

//Create a single instance initialized (perhaps partially) with entries
func NewBATBlockWithEntries(bigBlockSize POIFSBigBlockSize, entries []int, start_index, end_index int) *BATBlock {
	bb := NewBATBlock(bigBlockSize)
	for k := start_index; k < end_index; k++ {
		bb.values[k-start_index] = entries[k]
	}

	//Do we have any free sectors?
	if end_index-start_index == len(bb.values) {
		bb.recomputerFree()
	}
	return bb
}

func CreateBATBlock(bigBlockSize POIFSBigBlockSize, data bytes.Buffer) *BATBlock {
	block := NewBATBlock(bigBlockSize)
	buff := make([]byte, INT_SIZE)
	for i := 0; i < len(block.values); i++ {
		data.Read(buff)
		block.values[i] = getInt(buff, 0)
	}
	block.recomputerFree()
	// All done
	return block
}

func CreateEmptyBATBlock(bigBlockSize POIFSBigBlockSize, isXBAT bool) *BATBlock {
	block := NewBATBlock(bigBlockSize)
	if isXBAT {
		block.setXBATChain(bigBlockSize, END_OF_CHAIN)
	}
	return block
}

func CreateBATBlocks(bigBlockSize POIFSBigBlockSize, entries []int) []*BATBlock {
	block_count := calculateStorageRequirements(bigBlockSize, len(entries))
	blocks := make([]*BATBlock, block_count)
	index := 0
	remaining := len(entries)

	enteries_per_block := bigBlockSize.BATEntriesPerBlock()

	for j := 0; j < len(entries); j += enteries_per_block {
		var eIdx int
		if remaining > enteries_per_block {
			eIdx = j + enteries_per_block
		} else {
			eIdx = len(entries)
		}
		blocks[index] = NewBATBlockWithEntries(bigBlockSize, entries, j, eIdx)
		index += 1
		remaining -= enteries_per_block
	}

	return blocks
}

func CreateXBATBlocks(bigBlockSize POIFSBigBlockSize, entries []int, startBlock int) []*BATBlock {
	block_count := calculateXBATStorageRequirements(bigBlockSize, len(entries))
	blocks := make([]*BATBlock, block_count)
	index := 0
	remaining := len(entries)

	_entries_per_xbat_block := bigBlockSize.XBATEntriesPerBlock()
	if block_count != 0 {
		for j := 0; j < len(entries); j += _entries_per_xbat_block {
			var eIdx int
			if remaining > _entries_per_xbat_block {
				eIdx = j + _entries_per_xbat_block
			} else {
				eIdx = len(entries)
			}
			blocks[index] = NewBATBlockWithEntries(bigBlockSize, entries, j, eIdx)
			index += 1
			remaining -= _entries_per_xbat_block
		}

		for index := 0; index < len(blocks)-1; index++ {
			blocks[index].setXBATChain(bigBlockSize, startBlock+index+1)
		}
		blocks[len(blocks)-1].setXBATChain(bigBlockSize, END_OF_CHAIN)
	}

	return blocks
}

func (bb *BATBlock) recomputerFree() {
	hasFree := false
	for k := 0; k < len(bb.values); k++ {
		if bb.values[k] == UNUSED_BLOCK {
			hasFree = true
			break
		}
	}
	bb.has_free_sectors = hasFree
}

func (bb *BATBlock) GetValueAt(offset int) (int, error) {
	if offset > len(bb.values) {
		return 0, errors.New(fmt.Sprintf("Unable to fetch offset %d as the BAT only contains %d entries",
			offset, len(bb.values)))
	}
	return bb.values[offset], nil
}

func (bb *BATBlock) SetValueAt(offset, val int) error {
	if offset > len(bb.values) {
		return errors.New(fmt.Sprintf("Unable to Set offset %d as the BAT only contains %d entries",
			offset, len(bb.values)))
	}
	oldValue := bb.values[offset]
	bb.values[offset] = val

	//Do we need to re-cmputer the free?
	if val == UNUSED_BLOCK {
		bb.has_free_sectors = true
		return nil
	}
	if oldValue == UNUSED_BLOCK {
		bb.recomputerFree()
	}
	return nil
}

func (bb *BATBlock) SetOurBlockIndex(index int) {
	bb.ourBlockIndex = index
}

func (bb *BATBlock) GetOutBlockIndex() int {
	return bb.ourBlockIndex
}

func (bb *BATBlock) setXBATChain(bigBlockSize POIFSBigBlockSize, chainIndex int) {
	bb.values[bigBlockSize.XBATEntriesPerBlock()] = chainIndex
}

func (bb *BATBlock) HasFreeSectors() bool {
	return bb.has_free_sectors
}

func calculateStorageRequirements(bigBlockSize POIFSBigBlockSize, entryCount int) int {
	return (entryCount + bigBlockSize.BATEntriesPerBlock() - 1) / bigBlockSize.BATEntriesPerBlock()
}

func calculateXBATStorageRequirements(bigBlockSize POIFSBigBlockSize, entryCount int) int {
	return (entryCount + bigBlockSize.XBATEntriesPerBlock() - 1) / bigBlockSize.XBATEntriesPerBlock()
}

/**
 * Calculates the maximum size of a file which is addressable given the
 *  number of FAT (BAT) sectors specified. (We don't care if those BAT
 *  blocks come from the 109 in the header, or from header + XBATS, it
 *  won't affect the calculation)
 *
 * The actual file size will be between [size of fatCount-1 blocks] and
 *   [size of fatCount blocks].
 *  For 512 byte block sizes, this means we may over-estimate by up to 65kb.
 *  For 4096 byte block sizes, this means we may over-estimate by up to 4mb
 */

func calculateMaximumSize(bigBlockSize POIFSBigBlockSize, numBAT int) int {
	size := -1 //Header isn't FAT addressed

	// The header has up to 109 BATs, and extra ones are referenced
	// from XBAT
	// However, all BATs can contain 128/1024 blocks
	size += (numBAT * bigBlockSize.BATEntriesPerBlock())

	// So far we've been in sector counts, turn into bytes

	return size * bigBlockSize.BigBlockSize
}

func calculateMaximumSizeFromHeader(header *HeaderBlock) int {
	return calculateMaximumSize(header.bigBlockSize, header.GetBATCount())
}

/**
 * Returns the BATBlock that handles the specified offset,
 *  and the relative index within it.
 * The List of BATBlocks must be in sequential order
 */

func GetSBATBlockAndIndex(offset int, header HeaderBlock, bats []*BATBlock) *BATBlockAndIndex {
	// SBATs are so much easier, as they're chained streams
	whichSBAT := int(math.Floor(float64(offset) / float64(header.bigBlockSize.BATEntriesPerBlock())))
	var index int = offset % header.bigBlockSize.BATEntriesPerBlock()
	return batBlockAndIndex(index, bats[whichSBAT])
}

/**
 * Returns the BATBlock that handles the specified offset,
 *  and the relative index within it, for the mini stream.
 * The List of BATBlocks must be in sequential order
 */
func GetBATBlockAndIndex(offset int, header HeaderBlock, bats []*BATBlock) *BATBlockAndIndex {
	whichBAT := int(math.Floor(float64(offset) / float64(header.bigBlockSize.BATEntriesPerBlock())))
	var index int = offset % header.bigBlockSize.BATEntriesPerBlock()
	return batBlockAndIndex(index, bats[whichBAT])
}

func (bb *BATBlock) WriteBlocks(w io.Writer) error {
	_, err := w.Write(bb.serialize())
	return err
}

func (bb *BATBlock) serialize() []byte {
	//create empty array
	data := make([]byte, bb.bigBlockSize.BigBlockSize)

	//Fill in the values
	offset := 0
	for i := 0; i < len(bb.values); i++ {
		putInt(data, offset, bb.values[i])
		offset += INT_SIZE
	}

	//Done
	return data
}

func batBlockAndIndex(index int, block *BATBlock) *BATBlockAndIndex {
	return &BATBlockAndIndex{index: index, block: block}
}

func (bi *BATBlockAndIndex) GetIndex() int {
	return bi.index
}

func (bi *BATBlockAndIndex) GetBlock() *BATBlock {
	return bi.block
}

type BlockAllocationTableWriter struct {
	entries      []int
	blocks       []*BATBlock
	start_block  int
	bigBlockSize POIFSBigBlockSize
}

func NewBlockAllocationTableWriter(bigBlockSize POIFSBigBlockSize) *BlockAllocationTableWriter {
	return &BlockAllocationTableWriter{bigBlockSize: bigBlockSize,
		start_block: END_OF_CHAIN,
		entries:     make([]int, 0),
		blocks:      make([]*BATBlock, 0),
	}
}

func (b *BlockAllocationTableWriter) CreateBlocks() int {
	xbat_blocks, bat_blocks := 0, 0

	for {
		calculated_bat_blocks := calculateStorageRequirements(b.bigBlockSize,
			bat_blocks+xbat_blocks+len(b.entries))

		calculated_xbat_blocks := calculateXBATStorageRequirementsForHB(b.bigBlockSize,
			calculated_bat_blocks)

		if bat_blocks == calculated_bat_blocks &&
			xbat_blocks == calculated_xbat_blocks {
			// stable ... we're OK
			break
		}
		bat_blocks = calculated_bat_blocks
		xbat_blocks = calculated_xbat_blocks
	}
	startBlock := b.allocateSpace(bat_blocks)
	b.allocateSpace(xbat_blocks)
	b.simpleCreateBlocks()
	return startBlock
}

func (b *BlockAllocationTableWriter) allocateSpace(blockCount int) int {
	startBlock := len(b.entries)
	if blockCount > 0 {
		limit := blockCount - 1
		index := startBlock + 1

		for k := 0; k < limit; k++ {
			b.entries = append(b.entries, index)
			index += 1
		}
		b.entries = append(b.entries, END_OF_CHAIN)
	}
	return startBlock
}

func (b *BlockAllocationTableWriter) GetStartBlock() int {
	return b.start_block
}

func (b *BlockAllocationTableWriter) simpleCreateBlocks() {
	b.blocks = CreateBATBlocks(b.bigBlockSize, b.entries)
}

func (b *BlockAllocationTableWriter) WriteBlocks(stream io.Writer) error {
	for _, b := range b.blocks {
		if err := b.WriteBlocks(stream); err != nil {
			return err
		}
	}
	return nil
}

func (b *BlockAllocationTableWriter) CountBlocks() int {
	return len(b.blocks)
}

func (b *BlockAllocationTableWriter) SetStartBlock(start_block int) {
	b.start_block = start_block
}

type SmallBlockTableWriter struct {
	sbat            *BlockAllocationTableWriter
	small_blocks    []BlockWritable
	big_block_count int
	root            *RootProperty
}

func NewSmalBlockTableWriter(bigBlockSize POIFSBigBlockSize, documents []*POIFSDocument,
	root *RootProperty) *SmallBlockTableWriter {

	w := &SmallBlockTableWriter{
		sbat:         NewBlockAllocationTableWriter(bigBlockSize),
		small_blocks: make([]BlockWritable, 0),
		root:         root,
	}
	for _, doc := range documents {
		blocks := doc.GetSmallBlocks()
		if len(blocks) != 0 {
			doc.SetStartBlock(w.sbat.allocateSpace(len(blocks)))
			for j := 0; j < len(blocks); j++ {
				w.small_blocks = append(w.small_blocks, blocks[j])
			}
		} else {
			doc.SetStartBlock(END_OF_CHAIN)
		}
	}

	w.sbat.simpleCreateBlocks()
	w.root.SetSize(len(w.small_blocks))
	w.big_block_count = SmallDocumentBlockFill(bigBlockSize, &w.small_blocks)
	return w
}

func (s *SmallBlockTableWriter) GetSBATBlockCount() int {
	return (s.big_block_count + 15) / 16
}

func (s *SmallBlockTableWriter) GetSBAT() *BlockAllocationTableWriter {
	return s.sbat
}

func (s *SmallBlockTableWriter) CountBlocks() int {
	return s.big_block_count
}

func (s *SmallBlockTableWriter) SetStartBlock(start_block int) {
	s.root.SetStartBlock(start_block)
}

func (s *SmallBlockTableWriter) WriteBlocks(w io.Writer) error {
	for _, s := range s.small_blocks {
		if err := s.WriteBlocks(w); err != nil {
			return err
		}
	}
	return nil
}
