package adapter

import (
	"hash/fnv"
	"strings"
	"unicode"
)

// SparseVector 是 Qdrant 稀疏向量(indices 为 token 哈希,values 为词频)。
// collection 侧配置 modifier=idf 后,Qdrant 检索时按 IDF 加权计分,近似 BM25。
type SparseVector struct {
	Indices []uint32  `json:"indices"`
	Values  []float32 `json:"values"`
}

// BuildSparseVector 把文本转成 TF 稀疏向量:分词 → 统计词频 → token 哈希为维度下标。
// 空文本或无有效 token 时返回 nil(调用方据此跳过关键词召回)。
func BuildSparseVector(text string) *SparseVector {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return nil
	}
	tf := make(map[uint32]float32, len(tokens))
	for _, t := range tokens {
		tf[hashToken(t)]++
	}
	sv := &SparseVector{
		Indices: make([]uint32, 0, len(tf)),
		Values:  make([]float32, 0, len(tf)),
	}
	for idx, v := range tf {
		sv.Indices = append(sv.Indices, idx)
		sv.Values = append(sv.Values, v)
	}
	return sv
}

// tokenize 做中英混合分词:
//   - 连续的字母/数字作为一个 token(小写化),覆盖英文单词、标识符、数字;
//   - CJK 字符按 bigram 切(单字歧义大,bigram 是无词典分词的常用折中);
//   - 其余字符视为分隔。
func tokenize(text string) []string {
	var tokens []string
	var word []rune // 当前累积的字母/数字串
	var cjkPrev rune

	flushWord := func() {
		if len(word) > 0 {
			tokens = append(tokens, strings.ToLower(string(word)))
			word = word[:0]
		}
	}

	for _, r := range text {
		switch {
		case unicode.IsLetter(r) && isCJK(r):
			flushWord()
			if cjkPrev != 0 {
				tokens = append(tokens, string([]rune{cjkPrev, r}))
			}
			cjkPrev = r
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			cjkPrev = 0
			word = append(word, r)
		default:
			flushWord()
			cjkPrev = 0
		}
	}
	flushWord()
	return tokens
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= 0x3040 && r <= 0x30FF) || // 日文假名
		(r >= 0xAC00 && r <= 0xD7AF) // 韩文
}

func hashToken(t string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(t))
	return h.Sum32()
}
