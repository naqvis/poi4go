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
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
)

var (
	CLOSED_STREAM_ERROR = errors.New("cannot perform requested operation on a closed stream")
)

type BATManaged interface {
	CountBlocks() int
	SetStartBlock(index int)
}

type POIFSFileSystem struct {
	root           *DirectoryNode
	property_table *PropertyTable
	documents      []*POIFSDocument
	bigBlockSize   POIFSBigBlockSize
}

type POIFSDocument struct {
	property        *DocumentProperty
	size            int
	bigBigBlockSize POIFSBigBlockSize
	small_store     *smallBlockStore
	big_store       *bigBlockStore
}

type POIFSDocumentPath struct {
	components []string
	hascode    int
}

//Create POIFS FileSystem intended for writing
func NewFileSystem() *POIFSFileSystem {
	fs := new(POIFSFileSystem)
	fs.bigBlockSize = SMALLER_BIG_BLOCK_SIZE_DETAILS

	header_block := NewHeaderBlock(fs.bigBlockSize)
	fs.property_table = NewPropertyTable(header_block)
	fs.root = nil
	fs.documents = make([]*POIFSDocument, 0)
	return fs
}

//Create Filesystem from Reader. Reader is read until EOF.
func FileSystemFromReader(r io.Reader) (*POIFSFileSystem, error) {
	fs := NewFileSystem()

	// read the header block from the stream
	header_block, err := NewHeaderBlockFromReader(r)
	if err != nil {
		return nil, err
	}
	fs.bigBlockSize = header_block.bigBlockSize

	// read the rest of the stream into blocks
	data_blocks, err := RawDataBlockList(r, fs.bigBlockSize)
	if err != nil {
		return nil, err
	}

	// set up the block allocation table (necessary for the
	// data_blocks to be manageable
	_, err = NewBlockAllocationTableReader(header_block.bigBlockSize,
		header_block.GetBATCount(),
		header_block.GetBATArray(),
		header_block.GetXBATCount(),
		header_block.GetXBATIndex(),
		data_blocks)
	if err != nil {
		return nil, err
	}

	// get property table from the document
	properties, err := NewPropertyTableWithList(header_block, data_blocks)
	if err != nil {
		return nil, err
	}

	//init documents
	smallDocumentBlocks, err := GetSmallDocumentBlocks(fs.bigBlockSize, data_blocks, properties.GetRoot().(*RootProperty),
		header_block.GetSBATStart())

	if err != nil {
		return nil, err
	}

	err = fs.processProperties(smallDocumentBlocks, data_blocks, properties.GetRoot().GetChildren(), nil, header_block.GetPropertyStart())

	if err != nil {
		return nil, err
	}

	// For whatever reason CLSID of root is always 0.
	root, _ := fs.GetRoot()
	root.SetStorageClsId(properties.GetRoot().GetStorageClsid())
	return fs, nil

}

/*
func New(r io.Reader) (*POIFSFileSystem, error) {
	fs := new(POIFSFileSystem)
	fs.rs = r
	var err error
	fs.header, err = NewHeaderBlock(fs.rs)
	if err != nil {
		return nil, err
	}
	fs.property_table = NewPropertyTable(fs.header)
	fs.root = nil
	fs.documents = make([]*POIFSDocument, 0)

	data_blocks, err := RawDataBlockList(fs.rs, fs.header.bigBlockSize)
	if err != nil {
		return nil, err
	}
	NewBlockAllocationTableReader(fs.header.bigBlockSize, int(fs.header.NumFatSectors),
		fs.header.InitialDifats[0:], int(fs.header.NumDifatSectors), int(fs.header.DifatSectorLoc), data_blocks)

	properties := NewPropertyTableWithList(fs.header, data_blocks)
	smallDocumentBlocks, err := GetSmallDocumentBlocks(fs.header.bigBlockSize, data_blocks, properties.GetRoot().(*RootProperty),
		int(fs.header.MiniFatSectorLoc))

	if err != nil {
		return nil, err
	}

	err = fs.processProperties(smallDocumentBlocks, data_blocks, properties.GetRoot().GetChildren(), nil, fs.header.GetPropertyStart())

	if err != nil {
		return nil, err
	}
	root, _ := fs.GetRoot()
	root.SetStorageClsId(properties.GetRoot().GetStorageClsid())
	return fs, nil
}
*/

func (fs *POIFSFileSystem) processProperties(small_blocks, big_blocks BlockList, props []IProperty,
	dir *DirectoryNode, headerPropertiesStartAt int) error {

	for _, prop := range props {
		name := prop.GetName()
		var parent *DirectoryNode
		if dir == nil {
			parent, _ = fs.GetRoot()
		} else {
			parent = dir
		}

		if prop.IsDirectory() {
			new_dir := parent.CreateDirectory(name)
			new_dir.SetStorageClsId(prop.GetStorageClsid())
			fs.processProperties(small_blocks, big_blocks, prop.(*DirectoryProperty).GetChildren(), new_dir.(*DirectoryNode),
				headerPropertiesStartAt)
		} else {
			startBlock := prop.GetStartBlock()
			size := prop.GetSize()
			var doc *POIFSDocument
			if prop.UseSmallBlocks() {
				blocks, err := small_blocks.Fetch(startBlock, headerPropertiesStartAt)
				if err != nil {
					return err
				}
				doc = NewPOIFSDocFromManagedBlock2(name, blocks, size)
			} else {
				blocks, err := big_blocks.Fetch(startBlock, headerPropertiesStartAt)
				if err != nil {
					return err
				}
				doc = NewPOIFSDocFromManagedBlock2(name, blocks, size)
			}
			parent.createDocument(doc)
		}
	}
	return nil
}

func (fs *POIFSFileSystem) Remove(entry Entry) {
	fs.Remove(entry)
	if entry.IsDirectory() {
		fs.deleteDocument(entry.(*DocumentNode).GetDocument())
	}
}

func (fs *POIFSFileSystem) GetRoot() (*DirectoryNode, error) {
	if fs.root == nil {
		var err error
		fs.root, err = NewDirectoryNode(fs.property_table.GetRoot(), fs, nil)
		if err != nil {
			return nil, err
		}
	}
	return fs.root, nil
}

func (fs *POIFSFileSystem) AddDirectory(prop ParentProperty) {
	fs.property_table.AddProperty(prop)
}

func (fs *POIFSFileSystem) AddDocument(doc *POIFSDocument) {
	fs.documents = append(fs.documents, doc)
	fs.property_table.AddProperty(doc.property)
}

func (fs *POIFSFileSystem) deleteDocument(doc *POIFSDocument) {
	found := false
	var idx int
	var d *POIFSDocument
	for idx, d = range fs.documents {
		if d == doc {
			found = true
			break
		}
	}

	if found {
		//Delete Entry
		fs.documents = append(fs.documents[:idx], fs.documents[idx+1:]...)
		//Avoid potential memory leak, by nil out the pointer values
		fs.documents[len(fs.documents)-1] = nil
		fs.documents = fs.documents[:len(fs.documents)-1]
	}
}

func (fs *POIFSFileSystem) CreateDir(name string) (DirectoryEntry, error) {
	root, err := fs.GetRoot()
	if err != nil {
		return nil, err
	}

	return root.CreateDirectory(name), nil
}

//func (fs *POIFSFileSystem) CreateDocument(name string, data []byte) (DocumentEntry, error) {
func (fs *POIFSFileSystem) CreateDocument(name string, r io.Reader) (DocumentEntry, error) {
	root, err := fs.GetRoot()
	if err != nil {
		return nil, err
	}

	//return root.CreateDocument(name, data), nil
	return root.CreateDocument(name, r), nil
}

func (fs *POIFSFileSystem) WriteFileSystem(output io.Writer) error {
	// get the property table ready
	fs.property_table.PreWrite()

	// create the small block store, and the SBAT
	root := fs.property_table.GetRoot().(*RootProperty)
	sbtw := NewSmalBlockTableWriter(fs.bigBlockSize, fs.documents, root)
	//fs.property_table.GetRoot().(*RootProperty))

	//create the block allocation table
	bat := NewBlockAllocationTableWriter(fs.bigBlockSize)

	// create a list of Manged Objects; the document plus the
	// property table and the small block table

	//var bm_objects []interface{} = []interface{}{fs.documents, fs.property_table,
	//	sbtw, sbtw.GetSBAT()}

	var bm_objects []BATManaged = make([]BATManaged, 0)
	for _, doc := range fs.documents {
		bm_objects = append(bm_objects, doc)
	}
	bm_objects = append(bm_objects, fs.property_table, sbtw, sbtw.GetSBAT())

	// walk the list, allocating space for each and assigning each
	// a starting block number

	for _, bmo := range bm_objects {
		//bmo := obj.(BATManaged)
		block_count := bmo.CountBlocks()
		if block_count != 0 {
			bmo.SetStartBlock(bat.allocateSpace(block_count))
		} else {
			// Either the BATManaged object is empty or its data
			// is composed of SmallBlocks; in either case,
			// allocating space in the BAT is inappropriate
		}
	}

	// allocate space for the block allocation table and take its
	// starting block
	batStartBlock := bat.CreateBlocks()

	// get the extended block allocation table blocks
	header_block_writer := NewHeaderBlockWriter(fs.bigBlockSize)

	xbat_blocks := header_block_writer.SetBATBlocks(bat.CountBlocks(), batStartBlock)

	// set the property table start block
	header_block_writer.SetPropertyStart(fs.property_table.GetStartBlock())

	// set the small block allocation table start block
	header_block_writer.SetSBATStart(sbtw.GetSBAT().GetStartBlock())

	// set the small block allocation table block count
	header_block_writer.SetSBATBlockCount(sbtw.GetSBATBlockCount())

	// the header is now properly initialized. Make a list of
	// writers (the header block, followed by the documents, the
	// property table, the small block store, the small block
	// allocation table, the block allocation table, and the
	// extended block allocation table blocks)

	var writers []BlockWritable = []BlockWritable{header_block_writer}
	for _, doc := range fs.documents {
		writers = append(writers, doc)
	}
	//a := make([]interface{}, 0)
	//a = append(a, sbtw.GetSBAT(), bat)
	//fmt.Sprintf("%v", a)
	writers = append(writers, fs.property_table, sbtw, sbtw.GetSBAT(), bat)

	for j := 0; j < len(xbat_blocks); j++ {
		writers = append(writers, xbat_blocks[j])
	}

	//now, write everything out
	for _, writer := range writers {
		if err := writer.WriteBlocks(output); err != nil {
			return err
		}
	}
	return nil
}

func CreatePOIFSDocumentPath() (*POIFSDocumentPath, error) {
	dp := new(POIFSDocumentPath)
	dp.components = make([]string, 0)
	return dp, nil
}

func NewPOIFSDocumentPath(components []string) (*POIFSDocumentPath, error) {
	dp := new(POIFSDocumentPath)
	if components == nil || len(components) <= 0 {
		dp.components = make([]string, 0)
	} else {
		dp.components = make([]string, 0, len(components))
		for _, comp := range components {
			if len(comp) == 0 {
				return nil, errors.New("components cannot contain null or empty strings")
			}
			dp.components = append(dp.components)
		}
	}

	return dp, nil
}

func AddPathToPOIFSDocumentPath(path *POIFSDocumentPath, components []string) (*POIFSDocumentPath, error) {
	dp := new(POIFSDocumentPath)
	if components == nil || len(components) <= 0 {
		dp.components = make([]string, 0, len(path.components))
	} else {
		dp.components = make([]string, 0, len(path.components)+len(components))
	}
	for _, comp := range path.components {
		dp.components = append(dp.components, comp)
	}

	if components != nil && len(components) > 0 {
		for _, comp := range components {
			if len(comp) == 0 {
				log.Printf("Warning! Directory under %s has an empty name, "+
					"not all OLE2 readers will handle this file correctly!", path)
				//return nil, errors.New("components cannot contain null or empty strings")
			}
			dp.components = append(dp.components, comp)
		}
	}
	return dp, nil
}

func (dp *POIFSDocumentPath) Length() int {
	return len(dp.components)
}

func (dp *POIFSDocumentPath) Hashcode() int {
	if dp.hascode == 0 {
		for _, c := range dp.components {
			dp.hascode += hashCode(c)
		}
	}
	return dp.hascode
}

func (dp *POIFSDocumentPath) GetComponent(n int) string {
	if n < 0 || n > len(dp.components) {
		return ""
	}
	return dp.components[n]
}

func (dp *POIFSDocumentPath) GetParent() *POIFSDocumentPath {
	length := len(dp.components) - 1
	if length < 0 {
		return nil
	}
	parent, err := NewPOIFSDocumentPath(nil)
	if err != nil {
		return nil
	}
	parent.components = make([]string, length)
	copy(parent.components[0:], dp.components[0:length])
	return parent
}

func (dp *POIFSDocumentPath) String() string {
	return strings.Join(dp.components, string(os.PathSeparator))
}

func NewPOIFSDocFromRaw(name string, blocks []*rawDataBlock, length int) *POIFSDocument {
	doc := new(POIFSDocument)
	doc.size = length
	if len(blocks) == 0 {
		doc.bigBigBlockSize = SMALLER_BIG_BLOCK_SIZE_DETAILS
	} else {
		if blocks[0].BigBlockSize() == SMALLER_BIG_BLOCK_SIZE {
			doc.bigBigBlockSize = SMALLER_BIG_BLOCK_SIZE_DETAILS
		} else {
			doc.bigBigBlockSize = LARGER_BIG_BLOCK_SIZE_DETAILS
		}
	}
	doc.big_store = createBigBlockStore(doc.bigBigBlockSize, doc.convertRawBlocksToBigBlocks(blocks))
	doc.property = NewDocProperty(name, doc.size)
	doc.small_store = createSmallBlockStore(doc.bigBigBlockSize, empty_small_block_array)
	//doc.property.document = doc
	doc.property.SetDocument(doc)

	return doc
}

func NewPOIFSDocFromSmallBlocks(name string, blocks []*smallDocumentBlock, length int) *POIFSDocument {
	doc := new(POIFSDocument)
	doc.size = length
	if len(blocks) == 0 {
		doc.bigBigBlockSize = SMALLER_BIG_BLOCK_SIZE_DETAILS
	} else {
		doc.bigBigBlockSize = blocks[0].bigBlockSize
	}

	doc.big_store = createBigBlockStore(doc.bigBigBlockSize, empty_big_block_array)
	doc.property = NewDocProperty(name, doc.size)
	doc.small_store = createSmallBlockStore(doc.bigBigBlockSize, blocks)
	//doc.property.document = doc
	doc.property.SetDocument(doc)
	return doc
}

func NewPOIFSDocFromManagedBlock(name string, bigBlockSize POIFSBigBlockSize, blocks []ListManagedBlock, length int) *POIFSDocument {
	doc := new(POIFSDocument)
	doc.size = length
	doc.bigBigBlockSize = bigBlockSize
	doc.property = NewDocProperty(name, doc.size)
	//doc.property.document = doc
	doc.property.SetDocument(doc)

	if isSmall(doc.size) {
		doc.big_store = createBigBlockStore(bigBlockSize, empty_big_block_array)
		doc.small_store = createSmallBlockStore(bigBlockSize, doc.convertRawBlocksToSmallBlocks(blocks))
	} else {
		rbs := make([]*rawDataBlock, 0, len(blocks))
		for _, b := range blocks {
			rbs = append(rbs, b.(*rawDataBlock))
		}
		doc.big_store = createBigBlockStore(bigBlockSize, doc.convertRawBlocksToBigBlocks(rbs))
		doc.small_store = createSmallBlockStore(bigBlockSize, empty_small_block_array)

	}
	return doc
}

func NewPOIFSDocFromManagedBlock2(name string, blocks []ListManagedBlock, length int) *POIFSDocument {
	return NewPOIFSDocFromManagedBlock(name, SMALLER_BIG_BLOCK_SIZE_DETAILS, blocks, length)
}

//func NewPOIFSDocFromStream2(name string, bigBlockSize POIFSBigBlockSize, data []byte) (*POIFSDocument, error) {
func NewPOIFSDocFromStream2(name string, bigBlockSize POIFSBigBlockSize, r io.Reader) (*POIFSDocument, error) {
	blocks := make([]*DocumentBlock, 0)
	doc := new(POIFSDocument)
	doc.size = 0
	doc.bigBigBlockSize = bigBlockSize
	for true {
		//block, err := DocumentBlockFromStream(data, bigBlockSize)
		block, err := DocumentBlockFromStream(r, bigBlockSize)
		if err != nil {
			return nil, err
		}

		blockSize := block.Size()
		if blockSize > 0 {
			blocks = append(blocks, block)
			doc.size += blockSize
		}
		if block.PartiallyRead() {
			break
		}
	}

	doc.big_store = createBigBlockStore(bigBlockSize, blocks)
	doc.property = NewDocProperty(name, doc.size)
	//doc.property.document = doc
	doc.property.SetDocument(doc)
	if doc.property.UseSmallBlocks() {
		wblocks := make([]BlockWritable, 0, len(blocks))
		for _, b := range blocks {
			wblocks = append(wblocks, b)
		}

		doc.small_store = createSmallBlockStore(bigBlockSize, SmallDocBlockFromDataBlock(bigBlockSize, wblocks, doc.size))
		doc.big_store = createBigBlockStore(bigBlockSize, make([]*DocumentBlock, 0))
	} else {
		doc.small_store = createSmallBlockStore(bigBlockSize, empty_small_block_array)
	}
	return doc, nil
}

//func NewPOIFSDocFromStream(name string, data []byte) (*POIFSDocument, error) {
func NewPOIFSDocFromStream(name string, r io.Reader) (*POIFSDocument, error) {
	return NewPOIFSDocFromStream2(name, SMALLER_BIG_BLOCK_SIZE_DETAILS, r) // data)
}

func (d *POIFSDocument) convertRawBlocksToBigBlocks(blocks []*rawDataBlock) []*DocumentBlock {
	res := make([]*DocumentBlock, 0, len(blocks))
	for _, b := range blocks {
		db, err := DocumentBlockFromRawBlock(b)
		if err != nil {
			panic(err)
		}
		res = append(res, db)
	}
	return res
}

func (d *POIFSDocument) convertRawBlocksToSmallBlocks(blocks []ListManagedBlock) []*smallDocumentBlock {
	res := make([]*smallDocumentBlock, 0, len(blocks))
	for _, b := range blocks {
		if s, ok := b.(*smallDocumentBlock); ok {
			res = append(res, s)
		}
	}
	return res
}

func (d *POIFSDocument) GetDocumentProperty() *DocumentProperty {
	return d.property
}

func (d *POIFSDocument) GetDataInputBlock(offset int) *DataInputBlock {
	if offset >= d.size {
		if offset > d.size {
			panic("Request for offset " + strconv.Itoa(offset) +
				" doc size is " + strconv.Itoa(d.size))
		}
		return nil
	}
	if d.property.UseSmallBlocks() {
		return GetDataInputBlockFromSmallBlocks(d.small_store.smallBlocks, offset)
	}

	return GetDataInputBlockFromDocumentBlocks(d.big_store.bigBlocks, offset)
}

func (d *POIFSDocument) GetSmallBlocks() []BlockWritable {

	ret := make([]BlockWritable, 0, len(d.small_store.smallBlocks))
	for _, s := range d.small_store.smallBlocks {
		ret = append(ret, s)
	}

	return ret
}

func (d *POIFSDocument) SetStartBlock(index int) {
	d.property.SetStartBlock(index)
}

func (d *POIFSDocument) CountBlocks() int {
	return d.big_store.CountBlocks()
}

func (d *POIFSDocument) WriteBlocks(w io.Writer) error {
	return d.big_store.WriteBlocks(w)
}

type DocumentInputStream struct {
	current_offset, marked_offset, document_size int
	closed                                       bool
	document                                     *POIFSDocument
	currentBlock                                 *DataInputBlock
}

func NewDocInputStreamFromDocNode(doc *DocumentNode) *DocumentInputStream {
	//if doc.GetDocument() == nil {
	if doc.document == nil {
		panic("cannot open internal document storage")
	}

	dis := new(DocumentInputStream)
	dis.current_offset = 0
	dis.marked_offset = 0
	dis.document_size = doc.GetSize()
	dis.closed = false
	dis.document = doc.GetDocument()
	dis.currentBlock = dis.getDataInputBlock(0)
	return dis
}

func NewDocInputStreamFromEntry(entry DocumentEntry) *DocumentInputStream {
	//var doc *DocumentNode
	doc, ok := entry.(*DocumentNode)
	if !ok {
		panic("Not Document Node or can not open internal document storage")
	}
	return NewDocInputStreamFromDocNode(doc)
}

func NewDocInputStream(doc *POIFSDocument) *DocumentInputStream {
	dis := new(DocumentInputStream)
	dis.current_offset = 0
	dis.marked_offset = 0
	dis.document_size = doc.size
	dis.closed = false
	dis.document = doc
	dis.currentBlock = dis.getDataInputBlock(0)
	return dis
}

func (d *DocumentInputStream) Available() int {
	if d.closed {
		panic("Cannot perform requested operation on a closed stream")
	}

	return d.document_size - d.current_offset
}

func (d *DocumentInputStream) Close() {
	d.closed = true
}

func (d *DocumentInputStream) Mark(ignoreReadLimit int) {
	d.marked_offset = d.current_offset
}

func (d *DocumentInputStream) GetDataInputBlock(offset int) *DataInputBlock {
	return d.getDataInputBlock(offset)
}

func (d *DocumentInputStream) Reset() {
	d.current_offset = d.marked_offset
	d.currentBlock = d.getDataInputBlock(d.current_offset)
}

func (d *DocumentInputStream) getDataInputBlock(offset int) *DataInputBlock {
	return d.document.GetDataInputBlock(offset)
}
func (d *DocumentInputStream) atEOD() bool {
	return d.current_offset == d.document_size
}

func (d *DocumentInputStream) Read(p []byte) (n int, err error) {
	if d.closed {
		return 0, CLOSED_STREAM_ERROR
	}

	if len(p) == 0 {
		return 0, errors.New("Invalid buffer passed. Buffer length should > 0")
	}

	if d.atEOD() {
		return 0, io.EOF
	}

	limit := min(d.Available(), len(p))
	return d.readFully(p, limit)
}

func (d *DocumentInputStream) checkAvailable(size int) error {
	if d.closed {
		return CLOSED_STREAM_ERROR
	}
	if size > d.document_size-d.current_offset {
		return errors.New(fmt.Sprintf("Buffer underrun - requested %d bytes"+
			" but %d was available.", size, (d.document_size - d.current_offset)))
	}

	return nil
}

func (d *DocumentInputStream) readFully(buf []byte, length int) (int, error) {
	if err := d.checkAvailable(length); err != nil {
		return 0, err
	}
	blockAvailable := d.currentBlock.Available()
	if blockAvailable > length {
		d.currentBlock.ReadFully(buf, d.current_offset, length)
		d.current_offset += length
		return length, nil
	}
	remaining := length
	writePos := d.current_offset

	for remaining > 0 {
		blockIsExpiring := remaining >= blockAvailable
		var reqSize int
		if blockIsExpiring {
			reqSize = blockAvailable
		} else {
			reqSize = remaining
		}

		d.currentBlock.ReadFully(buf, writePos, reqSize)
		remaining -= reqSize
		writePos += reqSize
		d.current_offset += reqSize
		if blockIsExpiring {
			if d.current_offset == d.document_size {
				if remaining > 0 {
					return length - remaining, errors.New("reached end of document stream unexpectedly")
				}
				d.currentBlock = nil
				break
			}
			d.currentBlock = d.getDataInputBlock(d.current_offset)
			blockAvailable = d.currentBlock.Available()
		}
	}
	return length, nil
}
