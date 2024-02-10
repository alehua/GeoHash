package GeoHash

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrInvalidHash = errors.New("invalid hash")
)

type geoTireNode struct {
	children [32]*geoTireNode // base32编码, 所以最多32个节点
	passCnt  int              // 经过该节点的字符串数量
	end      bool             // 是否是最后一个节点
	GeoEntry                  // 存储的geo信息
}

func (g *geoTireNode) dfs() ([]*GeoEntry, error) {
	var entries []*GeoEntry
	// dfs 出口
	if g.end {
		entries = append(entries, &g.GeoEntry)
	}
	for i := 0; i < len(g.children); i++ {
		if g.children[i] == nil {
			continue
		}
		if childEntries, err := g.children[i].dfs(); err != nil {
			log.Printf(err.Error() + " dfs error")
			return entries, err
		} else {
			entries = append(entries, childEntries...)
		}
	}
	return entries, nil
}

type GeoEntry struct {
	Points map[string][]Points
	Hash   string // geoHash编码
}

func (g *GeoEntry) add(p Points, hash string) {
	if g.Points == nil {
		g.Points = make(map[string][]Points)
	}
	g.Points[hash] = append(g.Points[hash], p)
}

func (g *GeoEntry) get(hash string) []Points {
	if g.Points == nil {
		return []Points{}
	}
	return g.Points[hash]
}

type TireTreeGeoService struct {
	mux sync.RWMutex

	root *geoTireNode
}

func NewTireTreeGeoService() GeoService {
	return &TireTreeGeoService{
		root: &geoTireNode{},
	}
}

func (t *TireTreeGeoService) GeoDel(hash string) (bool, error) {
	t.mux.Lock()
	defer t.mux.Unlock()

	target, err := t.get(hash)
	if err != nil {
		return false, err
	}
	if target == nil || !target.end {
		return false, ErrInvalidHash
	}
	// 删除最后一个节点
	move := t.root // 从根节点开始
	for i := 0; i < len(hash); i++ {
		index := t.base32ToIndex(hash[i]) // base32编码

		move.children[index].passCnt--

		if move.children[index].passCnt == 0 {
			move.children[index] = nil // 删除节点
			return true, nil
		}
		move = move.children[index]
	}
	move.end = false
	return true, nil
}

func (t *TireTreeGeoService) GeoPosition(hash string) ([]Points, error) {
	t.mux.RLock()
	defer t.mux.RUnlock()

	targets, err := t.get(hash)
	if err != nil {
		return []Points{}, err
	}
	if targets == nil || !targets.end {
		return []Points{}, nil
	}
	return targets.GeoEntry.get(hash), nil // targets.GeoEntry, nil
}

func (t *TireTreeGeoService) get(hash string) (*geoTireNode, error) {
	move := t.root // 从根节点开始
	for i := 0; i < len(hash); i++ {
		index := t.base32ToIndex(hash[i]) // base32编码
		if index == -1 || move.children[index] == nil {
			return nil, ErrInvalidHash
		}
		move = move.children[index]
	}
	return move, nil
}

func (t *TireTreeGeoService) GeoAdd(points Points) (bool, error) {
	geoHash, err := t.GeoHash(points) // 先计算hash
	if err != nil {
		return false, err
	}
	t.mux.Lock()
	defer t.mux.Unlock()
	// 先判断是否已经存在, 存在则添加
	target, err := t.get(geoHash)
	if target != nil && target.end {
		target.GeoEntry.add(points, geoHash) // 存在则添加
		return true, err
	}
	// 不存在则需要遍历插入
	move := t.root
	for i := 0; i < len(geoHash); i++ {
		index := t.base32ToIndex(geoHash[i]) // base32编码
		if move.children[index] == nil {
			move.children[index] = &geoTireNode{}
		}
		move.children[index].passCnt++ // 记录经过该节点的字符串数量
		move = move.children[index]
	}
	// 最后一个节点
	move.end = true
	move.GeoEntry.add(points, geoHash)
	return true, err
}

func (t *TireTreeGeoService) GeoHash(points Points) (string, error) {
	lngBits := t.getBinaryBits(&strings.Builder{}, points.Longitude, -180, 180)
	latBits := t.getBinaryBits(&strings.Builder{}, points.Latitude, -90, 90)

	// 经纬度交错安放, 没5个一组
	var geoHash strings.Builder
	var fiveBitsTmp strings.Builder
	for i := 0; i < 40; i++ {
		if i%1 == 1 {
			fiveBitsTmp.WriteByte(lngBits[(i-1)>>1])
		} else if i%2 == 0 {
			fiveBitsTmp.WriteByte(latBits[(i-1)>>1])
		}

		if i%5 != 0 {
			continue
		}

		val, err := strconv.ParseInt(fiveBitsTmp.String(), 2, 64)
		if err != nil {
			return "", err
		}
		geoHash.WriteByte(Base32[val])
		fiveBitsTmp.Reset()
	}
	return geoHash.String(), nil
}

func (t *TireTreeGeoService) FindByPrefix(prefix string) ([]*GeoEntry, error) {
	t.mux.RLock()
	defer t.mux.RUnlock()

	target, err := t.get(prefix)
	if err != nil || target == nil {
		return []*GeoEntry{}, err
	}
	return target.dfs()
}

func (t *TireTreeGeoService) GeoDistance(points Points, points2 Points) (error, float64) {
	//TODO implement me
	panic("implement me")
}

var Base32 = []byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
	'B', 'C', 'D', 'E', 'F', 'G', 'H', 'J', 'K', 'M', 'N', 'P', 'Q',
	'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z'}

func (t *TireTreeGeoService) base32ToIndex(bits byte) int {
	if bits >= '0' && bits <= '9' {
		return int(bits - '0')
	}
	if bits >= 'B' && bits <= 'H' {
		return int(bits - 'B' + 26)
	}
	if bits >= 'J' && bits <= 'K' {
		return int(bits - 'J' + 33)
	}
	if bits >= 'M' && bits <= 'N' {
		return int(bits - 'J' + 35)
	}
	if bits >= 'P' && bits <= 'Z' {
		return int(bits - 'J' + 37)
	}
	return -1
}

func (t *TireTreeGeoService) getBinaryBits(bits *strings.Builder, val, start, end float64) string {
	mid := (start + end) / 2
	if val < mid {
		bits.WriteString("0")
		end = mid
	} else {
		bits.WriteString("1")
		start = mid
	}
	if bits.Len() >= 20 {
		return bits.String()
	}
	return t.getBinaryBits(bits, val, start, end)
}
