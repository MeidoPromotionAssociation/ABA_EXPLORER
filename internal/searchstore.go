package internal

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// indexCacheMagic 与 indexCacheVersion 标识磁盘缓存的格式
// 记录布局一改就要提版本号，旧缓存会被直接判为不可用而不是解出错位的数据
// indexCacheMagic and indexCacheVersion identify the on-disk cache format
// The version must be bumped whenever the record layout changes so an old cache is rejected outright
// instead of decoding into misaligned data
const (
	indexCacheMagic   = "ABAIDX"
	indexCacheVersion = 1
)

// fileStamp 是一个文件的身份指纹，用来判断缓存里的那一份是否还对得上磁盘
// 只比大小与修改时间：重新读一遍内容算哈希，代价和重建索引本身相当，那样缓存就没意义了
// fileStamp fingerprints a file so a cached copy can be checked against what is on disk
// Only the size and modification time are compared, because rereading the contents to hash them would cost
// about as much as rebuilding the index and would defeat the point of the cache
type fileStamp struct {
	Path    string
	Size    int64
	ModUnix int64
}

// stampFile 读取一个文件当前的指纹，路径为空或读不到时返回只带路径的零值
// stampFile reads the current fingerprint of a file and returns a path-only zero value when the path is
// empty or unreadable
func stampFile(path string) fileStamp {
	if path == "" {
		return fileStamp{}
	}
	stamp := fileStamp{Path: path}
	if info, err := os.Stat(path); err == nil {
		stamp.Size = info.Size()
		stamp.ModUnix = info.ModTime().UnixNano()
	}
	return stamp
}

// current 判断这份指纹是否仍与磁盘一致
// current reports whether this fingerprint still matches what is on disk
func (stamp fileStamp) current() bool {
	if stamp.Path == "" {
		return true
	}
	info, err := os.Stat(stamp.Path)
	if err != nil {
		return false
	}
	return info.Size() == stamp.Size && info.ModTime().UnixNano() == stamp.ModUnix
}

// indexedSource 是一个来源（一个容器加它配套的 .ct）去重后的全部名字，连同两个文件的指纹
// 缓存与增量更新都以它为单位：两个文件都没变就整组沿用，任一变了就只重扫这一组
// indexedSource holds every deduplicated name of one source, a container plus its paired .ct, along with
// the fingerprints of both files
// It is the unit of both caching and incremental updates: an untouched pair is reused whole, and a change
// to either file rescans only that one group
type indexedSource struct {
	containerName string
	container     fileStamp
	catalog       fileStamp
	entries       []scannedName
}

// current 判断这个来源的两个文件是否都没变过
// current reports whether both files of this source are unchanged
func (source *indexedSource) current() bool {
	return source.container.current() && source.catalog.current()
}

// indexCache 是一份可写回磁盘的索引快照 / indexCache is an index snapshot that can be written back to disk
type indexCache struct {
	Root     string
	Deep     bool
	Sources  []indexedSource
	Warnings []string
}

// indexCachePath 返回一个根目录对应的缓存文件路径
// 路径用哈希而不是原样拼接：资源目录可能很深、含中文或盘符，直接当文件名会超长或非法
// indexCachePath returns the cache file path for one root directory
// The path is hashed rather than embedded because a resource directory can be deep, non-ASCII, or carry a
// drive letter, any of which would make an over-long or invalid file name
func indexCachePath(root string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the user config directory: %w", err)
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root))))
	return filepath.Join(base, "ABA_EXPLORER", "search-index", hex.EncodeToString(sum[:8])+".bin"), nil
}

// writeIndexCache 把索引快照原子地写到磁盘
// 先写临时文件再改名：中途崩溃或断电只会留下一个临时文件，而不是一份解码到一半就报错的缓存
// writeIndexCache atomically writes an index snapshot to disk
// A temporary file is written and then renamed, so a crash or power loss leaves a stray temporary file
// rather than a cache that fails halfway through decoding
func writeIndexCache(cache *indexCache) error {
	path, err := indexCachePath(cache.Root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create the index cache directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "index-*.tmp")
	if err != nil {
		return fmt.Errorf("create a temporary index cache: %w", err)
	}
	name := temporary.Name()
	writer := &indexEncoder{writer: bufio.NewWriterSize(temporary, 1<<20)}
	writer.encode(cache)
	flushErr := writer.writer.Flush()
	closeErr := temporary.Close()
	if err := errors.Join(writer.err, flushErr, closeErr); err != nil {
		os.Remove(name)
		return fmt.Errorf("write the index cache: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return fmt.Errorf("replace the index cache: %w", err)
	}
	return nil
}

// readIndexCache 读回一个根目录的索引快照，缓存不存在、版本不符或内容损坏时返回错误
// readIndexCache reads back the index snapshot of a root directory and errors when the cache is absent,
// carries another version, or is corrupt
func readIndexCache(root string) (*indexCache, error) {
	path, err := indexCachePath(root)
	if err != nil {
		return nil, err
	}
	handle, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	reader := &indexDecoder{reader: bufio.NewReaderSize(handle, 1<<20)}
	cache := reader.decode()
	if reader.err != nil {
		return nil, fmt.Errorf("read the index cache: %w", reader.err)
	}
	// 解完之后必须正好到文件尾。多出来的字节说明这不是我们写下的那份文件，
	// 前缀恰好能解通并不代表内容可信，宁可判为损坏让上层重建
	// Decoding must land exactly at end of file. Extra bytes mean this is not the file we wrote, and a prefix
	// that happens to decode does not make the content trustworthy, so it is treated as corrupt and rebuilt
	if _, err := reader.reader.ReadByte(); err == nil {
		return nil, errors.New("read the index cache: unexpected trailing data")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read the index cache: %w", err)
	}
	return cache, nil
}

// removeIndexCache 删除一个根目录的缓存，缓存本来就不存在时不算错误
// removeIndexCache deletes the cache of a root directory and treats an already absent cache as success
func removeIndexCache(root string) error {
	path, err := indexCachePath(root)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// indexEncoder 把索引快照写成紧凑的变长二进制
// indexEncoder writes an index snapshot as compact variable-length binary
type indexEncoder struct {
	writer  *bufio.Writer
	scratch [binary.MaxVarintLen64]byte
	err     error
}

func (encoder *indexEncoder) raw(data []byte) {
	if encoder.err != nil {
		return
	}
	_, encoder.err = encoder.writer.Write(data)
}

func (encoder *indexEncoder) uint(value uint64) {
	size := binary.PutUvarint(encoder.scratch[:], value)
	encoder.raw(encoder.scratch[:size])
}

func (encoder *indexEncoder) int(value int64) {
	size := binary.PutVarint(encoder.scratch[:], value)
	encoder.raw(encoder.scratch[:size])
}

func (encoder *indexEncoder) str(value string) {
	encoder.uint(uint64(len(value)))
	if encoder.err != nil {
		return
	}
	_, encoder.err = encoder.writer.WriteString(value)
}

func (encoder *indexEncoder) stamp(stamp fileStamp) {
	encoder.str(stamp.Path)
	encoder.int(stamp.Size)
	encoder.int(stamp.ModUnix)
}

func (encoder *indexEncoder) encode(cache *indexCache) {
	encoder.raw([]byte(indexCacheMagic))
	encoder.uint(indexCacheVersion)
	encoder.str(cache.Root)
	deep := uint64(0)
	if cache.Deep {
		deep = 1
	}
	encoder.uint(deep)

	encoder.uint(uint64(len(cache.Warnings)))
	for _, warning := range cache.Warnings {
		encoder.str(warning)
	}

	encoder.uint(uint64(len(cache.Sources)))
	for i := range cache.Sources {
		source := &cache.Sources[i]
		encoder.str(source.containerName)
		encoder.stamp(source.container)
		encoder.stamp(source.catalog)
		encoder.uint(uint64(len(source.entries)))
		for _, entry := range source.entries {
			encoder.str(entry.name)
			encoder.str(entry.detail)
			encoder.str(entry.owner)
			encoder.uint(uint64(originID(entry.origin)))
		}
	}
}

// indexDecoder 读回 indexEncoder 写下的二进制
// indexDecoder reads back the binary written by indexEncoder
type indexDecoder struct {
	reader *bufio.Reader
	err    error
}

func (decoder *indexDecoder) uint() uint64 {
	if decoder.err != nil {
		return 0
	}
	value, err := binary.ReadUvarint(decoder.reader)
	if err != nil {
		decoder.err = err
	}
	return value
}

func (decoder *indexDecoder) int() int64 {
	if decoder.err != nil {
		return 0
	}
	value, err := binary.ReadVarint(decoder.reader)
	if err != nil {
		decoder.err = err
	}
	return value
}

// maxCachedStringLen 是缓存里单个字符串的长度上限
// 损坏的缓存可能读出一个天文数字的长度，没有这道闸就会直接申请几个 GB 然后崩掉
// maxCachedStringLen caps one string inside the cache
// A corrupt cache can yield an astronomical length, and without this gate the decoder would allocate
// gigabytes and die
const maxCachedStringLen = 1 << 20

func (decoder *indexDecoder) str() string {
	length := decoder.uint()
	if decoder.err != nil {
		return ""
	}
	if length > maxCachedStringLen {
		decoder.err = fmt.Errorf("cached string length %d exceeds the limit %d", length, maxCachedStringLen)
		return ""
	}
	if length == 0 {
		return ""
	}
	buffer := make([]byte, length)
	if _, err := io.ReadFull(decoder.reader, buffer); err != nil {
		decoder.err = err
		return ""
	}
	return string(buffer)
}

// maxCachedCount 是缓存里任何一个计数字段的上限，同样用来挡住损坏内容导致的巨量分配
// maxCachedCount caps every count field in the cache and likewise guards against huge allocations from
// corrupt content
const maxCachedCount = 1 << 26

func (decoder *indexDecoder) count(label string) int {
	value := decoder.uint()
	if decoder.err != nil {
		return 0
	}
	if value > maxCachedCount {
		decoder.err = fmt.Errorf("cached %s count %d exceeds the limit %d", label, value, maxCachedCount)
		return 0
	}
	return int(value)
}

func (decoder *indexDecoder) stamp() fileStamp {
	return fileStamp{Path: decoder.str(), Size: decoder.int(), ModUnix: decoder.int()}
}

func (decoder *indexDecoder) decode() *indexCache {
	magic := make([]byte, len(indexCacheMagic))
	if _, err := io.ReadFull(decoder.reader, magic); err != nil {
		decoder.err = err
		return nil
	}
	if string(magic) != indexCacheMagic {
		decoder.err = errors.New("this file is not an index cache")
		return nil
	}
	if version := decoder.uint(); decoder.err == nil && version != indexCacheVersion {
		decoder.err = fmt.Errorf("index cache version %d is not the expected %d", version, indexCacheVersion)
		return nil
	}

	cache := &indexCache{}
	cache.Root = decoder.str()
	cache.Deep = decoder.uint() == 1

	warningCount := decoder.count("warning")
	if decoder.err != nil {
		return nil
	}
	cache.Warnings = make([]string, 0, warningCount)
	for range warningCount {
		cache.Warnings = append(cache.Warnings, decoder.str())
	}

	sourceCount := decoder.count("source")
	if decoder.err != nil {
		return nil
	}
	cache.Sources = make([]indexedSource, 0, sourceCount)
	for range sourceCount {
		source := indexedSource{
			containerName: decoder.str(),
			container:     decoder.stamp(),
			catalog:       decoder.stamp(),
		}
		entryCount := decoder.count("entry")
		if decoder.err != nil {
			return nil
		}
		source.entries = make([]scannedName, 0, entryCount)
		for range entryCount {
			entry := scannedName{
				name:   decoder.str(),
				detail: decoder.str(),
				owner:  decoder.str(),
			}
			entry.origin = originName(uint8(decoder.uint()))
			if decoder.err != nil {
				return nil
			}
			source.entries = append(source.entries, entry)
		}
		cache.Sources = append(cache.Sources, source)
	}
	if decoder.err != nil {
		return nil
	}
	return cache
}
