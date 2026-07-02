package pinyin

import (
	"sort"
	"strings"
)

var syllables = map[string]bool{
	"a": true, "o": true, "e": true, "er": true, "ai": true, "ei": true,
	"ao": true, "ou": true, "an": true, "en": true, "ang": true, "eng": true,
	"ba": true, "bo": true, "bai": true, "bei": true, "bao": true, "ban": true,
	"ben": true, "bang": true, "beng": true, "bi": true, "bie": true, "biao": true,
	"bian": true, "bin": true, "bing": true, "bu": true, "biang": true,
	"pa": true, "po": true, "pai": true, "pei": true, "pao": true, "pan": true,
	"pen": true, "pang": true, "peng": true, "pi": true, "pie": true, "piao": true,
	"pian": true, "pin": true, "ping": true, "pou": true, "pu": true,
	"ma": true, "mo": true, "me": true, "mai": true, "mei": true, "mao": true,
	"man": true, "men": true, "mang": true, "meng": true, "mi": true, "mie": true,
	"miao": true, "mian": true, "min": true, "ming": true, "miu": true, "mou": true,
	"mu": true,
	"fa": true, "fo": true, "fei": true, "fan": true, "fen": true,
	"fang": true, "feng": true, "fou": true, "fu": true, "fiao": true,
	"da": true, "de": true, "dei": true, "den": true, "dai": true, "dia": true,
	"dao": true, "dan": true, "dang": true, "deng": true, "di": true, "die": true,
	"diao": true, "dian": true, "ding": true, "diu": true, "dong": true, "dou": true,
	"du": true, "duan": true, "dui": true, "dun": true, "duo": true,
	"ta": true, "te": true, "tei": true, "tai": true, "tao": true, "tan": true,
	"tang": true, "teng": true, "ti": true, "tie": true, "tiao": true, "tian": true,
	"ting": true, "tong": true, "tou": true, "tu": true, "tuan": true, "tui": true,
	"tun": true, "tuo": true,
	"na": true, "ne": true, "nai": true, "nei": true, "nao": true, "nan": true,
	"nen": true, "nang": true, "neng": true, "ni": true, "nie": true, "niao": true,
	"nian": true, "nin": true, "niang": true, "ning": true, "niu": true, "nong": true,
	"nou": true, "nu": true, "nuan": true, "nve": true, "nuo": true, "nv": true,
	"la": true, "le": true, "lai": true, "lei": true, "lao": true, "lan": true,
	"lo": true, "lou": true, "lang": true, "leng": true, "li": true, "lia": true,
	"lie": true, "liao": true, "lian": true, "lin": true, "liang": true, "ling": true,
	"liu": true, "long": true, "lu": true, "luan": true, "lun": true, "luo": true,
	"lve": true, "lv": true,
	"ga": true, "ge": true, "gei": true, "gai": true, "gao": true, "gan": true,
	"gen": true, "gang": true, "geng": true, "gong": true, "gou": true, "gu": true,
	"gua": true, "guai": true, "guan": true, "guang": true, "gui": true, "gun": true,
	"guo": true,
	"ka": true, "ke": true, "kei": true, "kai": true, "kao": true, "kan": true,
	"ken": true, "kang": true, "keng": true, "kong": true, "kou": true, "ku": true,
	"kua": true, "kuai": true, "kuan": true, "kuang": true, "kui": true, "kun": true,
	"kuo": true,
	"ha": true, "he": true, "hei": true, "hai": true, "hao": true, "han": true,
	"hen": true, "hang": true, "heng": true, "hong": true, "hou": true, "hu": true,
	"hua": true, "huai": true, "huan": true, "huang": true, "hui": true, "hun": true,
	"huo": true,
	"ji": true, "jia": true, "jie": true, "jiao": true, "jian": true, "jin": true,
	"jiang": true, "jing": true, "jiong": true, "jiu": true, "ju": true, "juan": true,
	"jue": true, "jun": true,
	"qi": true, "qia": true, "qie": true, "qiao": true, "qian": true, "qin": true,
	"qiang": true, "qing": true, "qiong": true, "qiu": true, "qu": true, "quan": true,
	"que": true, "qun": true,
	"xi": true, "xia": true, "xie": true, "xiao": true, "xian": true, "xin": true,
	"xiang": true, "xing": true, "xiong": true, "xiu": true, "xu": true, "xuan": true,
	"xue": true, "xun": true,
	"zhi": true, "zha": true, "zhe": true, "zhei": true, "zhai": true, "zhao": true,
	"zhan": true, "zhen": true, "zhang": true, "zheng": true, "zhong": true,
	"zhou": true, "zhu": true, "zhua": true, "zhuai": true, "zhuan": true,
	"zhuang": true, "zhui": true, "zhun": true, "zhuo": true,
	"chi": true, "cha": true, "che": true, "chai": true, "chao": true, "chan": true,
	"chen": true, "chang": true, "cheng": true, "chong": true, "chou": true,
	"chu": true, "chua": true, "chuai": true, "chuan": true, "chuang": true,
	"chui": true, "chun": true, "chuo": true,
	"shi": true, "sha": true, "she": true, "shei": true, "shai": true, "shao": true,
	"shan": true, "shen": true, "shang": true, "sheng": true, "shou": true,
	"shu": true, "shua": true, "shuai": true, "shuan": true, "shuang": true,
	"shui": true, "shun": true, "shuo": true,
	"ri": true, "ran": true, "rang": true, "rao": true, "re": true, "ren": true,
	"reng": true, "rong": true, "rou": true, "ru": true, "rua": true, "ruan": true,
	"rui": true, "run": true, "ruo": true,
	"zi": true, "za": true, "ze": true, "zei": true, "zai": true, "zao": true,
	"zan": true, "zen": true, "zang": true, "zeng": true, "zong": true, "zou": true,
	"zu": true, "zuan": true, "zui": true, "zun": true, "zuo": true,
	"ci": true, "ca": true, "ce": true, "cai": true, "cao": true, "can": true,
	"cen": true, "cang": true, "ceng": true, "cong": true, "cou": true, "cu": true,
	"cuan": true, "cui": true, "cun": true, "cuo": true,
	"si": true, "sa": true, "se": true, "cei": true, "sai": true, "sao": true, "san": true,
	"sen": true, "seng": true, "sang": true, "song": true, "sou": true, "su": true, "suan": true,
	"sui": true, "sun": true, "suo": true,
	"ya": true, "yo": true, "ye": true, "yan": true, "yang": true,
	"yao": true, "you": true, "yong": true, "yi": true, "yin": true, "ying": true,
	"yu": true, "yuan": true, "yue": true, "yun": true,
	"wa": true, "wo": true, "wai": true, "wei": true, "wan": true, "wang": true,
	"weng": true, "wu": true, "wen": true,
}

func isSyllable(s string) bool {
	return syllables[s]
}

func expandPrefix(prefix string) []string {
	var result []string
	for s := range syllables {
		if strings.HasPrefix(s, prefix) {
			result = append(result, s)
		}
	}
	sort.Strings(result)
	return result
}

func SplitFuzzy(input string) ([][]string, string) {
	if len(input) == 0 {
		return nil, ""
	}
	if strict := Split(input); len(strict) > 0 {
		return strict, ""
	}
	n := len(input)
	for cut := n - 1; cut >= 1; cut-- {
		prefix := input[:cut]
		partial := input[cut:]
		if strict := Split(prefix); len(strict) > 0 {
			expansions := expandPrefix(partial)
			if len(expansions) == 0 {
				continue
			}
			var results [][]string
			for _, base := range strict {
				for _, exp := range expansions {
					rest := Split(input[cut:])
					if len(rest) == 0 {
						continue
					}
					for _, r := range rest {
						if len(r) == 0 || r[0] != exp {
							continue
						}
						path := make([]string, 0, len(base)+len(r))
						path = append(path, base...)
						path = append(path, r...)
						results = append(results, path)
					}
				}
			}
			if len(results) > 0 {
				return results, partial
			}
			for _, base := range strict {
				for _, exp := range expansions {
					path := make([]string, 0, len(base)+1)
					path = append(path, base...)
					path = append(path, exp)
					results = append(results, path)
				}
			}
			if len(results) > 0 {
				return results, partial
			}
		}
	}
	expansions := expandPrefix(input)
	if len(expansions) > 0 {
		results := make([][]string, len(expansions))
		for i, exp := range expansions {
			results[i] = []string{exp}
		}
		return results, input
	}
	return nil, input
}

func Split(input string) [][]string {
	n := len(input)
	if n == 0 {
		return nil
	}
	memo := make(map[int][][]string)
	var dfs func(i int) [][]string
	dfs = func(i int) [][]string {
		if i == n {
			return [][]string{{}}
		}
		if v, ok := memo[i]; ok {
			return v
		}
		var results [][]string
		for j := n; j > i; j-- {
			prefix := input[i:j]
			if !isSyllable(prefix) {
				continue
			}
			for _, rest := range dfs(j) {
				path := make([]string, 0, len(rest)+1)
				path = append(path, prefix)
				path = append(path, rest...)
				results = append(results, path)
			}
		}
		memo[i] = results
		return results
	}
	res := dfs(0)
	return res
}
