package handler

import (
	"fmt"
	"testing"
)

func TestSmartSplitText_ShortText(t *testing.T) {
	// 短文本不分块
	text := "宁波弘泰 MSK 美东 ETD4.14 VAN 1750/2300"
	chunks := SmartSplitText(text)

	if len(chunks) != 1 {
		t.Errorf("期望1个分块，实际 %d 个", len(chunks))
	}

	if !chunks[0].HasPricing {
		t.Error("应该检测到价格信息")
	}

	fmt.Printf("短文本测试: %d 个分块\n", len(chunks))
}

func TestSmartSplitText_MultipleAgents(t *testing.T) {
	// 多个代理的长文本（需要超过800字才会分块）
	text := `美加线

1.宁波弘泰
MSK 美东 宁波出
ETD 04-10 MSK-TP11 MAERSK SHAMS/614E SAV 余10F/NWK 2F
SAV/ NWK 2700/3000++
HOU 2800/3100++
MIA/TAMPA 2900/3200++
以上为4.06-4.12进港价格
线下仓位无亏舱费，免用箱8 working days

2.宁波弘泰
ZIM-ZNS ETD4.17 MSC MUMBAI VIII V.GE615E(NJ3,18E)
NYC/BAL/MIA/JAX:2800/3500+A
进港有效期：4.8-4.14

ZIM-ZCP ETD4.19 ZIM MOUNT VINSON V.10E(ZV1,10E)
SAV/CHS/NFK:2720/3400+A
进港有效期：4.8-4.14

3.上海铮航
MSK WK15 4/6-4/12 上海出
美西 USD1900+AMS+换单费 船期参考：4/13
美东 USD2800+AMS+换单费 船期参考：4/9 4/14 4/16
美湾 USD2950+AMS+换单费 船期参考：4/8 4/15

4.上海顺圆
上海出
WHL 美西：1560/1950++，4-17号船：HMM AQUAMARINE/0011E
美东：2200/2750++，4-20号船： WAN HAI A10/E013`

	chunks := SmartSplitText(text)

	fmt.Printf("多代理测试: %d 个分块\n", len(chunks))
	for i, chunk := range chunks {
		fmt.Printf("分块 %d: Agent=%s, Region=%s, HasPricing=%v, 长度=%d\n",
			i+1, chunk.AgentName, chunk.Region, chunk.HasPricing, len(chunk.Content))
		if len(chunk.Content) > 100 {
			fmt.Printf("内容预览: %s\n\n", chunk.Content[:100])
		}
	}

	if len(chunks) < 2 {
		t.Errorf("期望至少2个分块，实际 %d 个", len(chunks))
	}
}

func TestSmartSplitText_SamePriceMultiplePOLs(t *testing.T) {
	// 5个起运港同价格的场景（不应该分块）
	text := `上海高阳

EMC 美线价格 青岛/上海/宁波/深圳/天津 同价
2026 4.8-4.14 进港

美西南 长滩 洛杉矶 奥克兰2260 2825 45尺3325（仅限CPS）
美西北 塔科马2260 2825 45尺3325  
美西南 长滩2260 2825 不接45尺 CEN航线
洛杉矶转内陆 芝加哥 达拉斯 孟菲斯 4550 5050不接45尺

美东 纽约 查尔斯顿 萨瓦那 巴尔的摩 波士顿 诺福克 3120 3900 45尺4500 
美湾 休斯顿 莫比尔 3600 4000 青岛不接45尺`

	chunks := SmartSplitText(text)

	fmt.Printf("多POL同价测试: %d 个分块\n", len(chunks))
	for i, chunk := range chunks {
		fmt.Printf("分块 %d: Agent=%s, Region=%s, 长度=%d\n",
			i+1, chunk.AgentName, chunk.Region, len(chunk.Content))
	}

	// 这个场景应该保持为1个分块，让模型处理多POL展开
	if len(chunks) > 2 {
		t.Errorf("多POL同价场景应该保持较少分块（1-2个），实际 %d 个", len(chunks))
	}
}

func TestSmartSplitText_LongInput1(t *testing.T) {
	// 用户输入1：包含多个代理和航线
	text := `中印红
1.宁波合天
宁波-吉布提
PIL--DJIBOUTI  4000/4800+ OCD 175/235  直达 目免17天 
4.8 KOTA SABAS V.0101W   进甬舟

BENLINE (边航）--DJIBOUTI 2900/4000 直达 目免21天
4.9  HONG DA XIN 68 V.26002W 进甬舟

JHS(吉航)-DJIBOUTI  3750/4200直达  目免21天
4.17  JI ZHE GLORY V.2603W 进甬舟

2.宁波科达
宁波出
ASCL 印巴 - ZHONG GU XI AN 2603W ETD SHA 4.8号  NGB  4.10  
NSA/MUN：1050/1150 14天
KHI：1225/1350  21天

3.宁波科达
RCL 红海调整RCR 红海线  SSF GALENE 2613W   4.11
JED SOK 3300/4400
SUDAN 3750/5500


欧基
1.宁波合天
CMA 宁波-欧基 运价更新 有效期 4.6-4.12 （按照实际开船日核算价格）
USD1475/2450+ENS
小柜含箱重20吨加超重费200
注意 CMA 含箱子大于等于32吨，THC 翻倍 宁波起步

2.宁波建航
ONE 单船集量 运价更新  更新！！！ 
上海出--4.10-FE4--ONE INTEGRITY/009W 
RTM/HAM/LEH : USD 1163/1850+ENS 
GDANSK/GDYNIA : USD1383/1850+ENS
LEIXOS/LISBON : USD1783/2350+ENS

3.宁波建航
ONE 欧洲单航次价格更新： 其他欧内陆都按照FAK
宁波出 
4.07/FE3----HMM OSLO/019W
HAM/FXT/ANT：1138/1800+ENS  NOR:1426+ENS   免用箱7+14
GDANSK/GDYNIA：1358/1800+ENS  NOR:1426+ENS 免用箱7+10

东南亚
1.宁波科达
YML
CTS GSL AFRICA 982S 4.6,预计晚到4.9
胡志明   375/700
金边      425/800
林查班   450/900
拉塔班   500/1000
 CTE DEAR PANEL 022S 4.5，预计晚到4.6
林查班   450/900
拉塔班   500/1000
YM IMAGE 221S 4.7
西哈努克   450/900
曼谷    575/1050`

	chunks := SmartSplitText(text)

	fmt.Printf("用户输入1测试: %d 个分块\n", len(chunks))
	for i, chunk := range chunks {
		fmt.Printf("分块 %d: Agent=%s, Region=%s, HasPricing=%v, 长度=%d\n",
			i+1, chunk.AgentName, chunk.Region, chunk.HasPricing, len(chunk.Content))
	}

	// 这个长文本应该分成多个块
	if len(chunks) < 3 {
		t.Errorf("长文本应该分成至少3个块，实际 %d 个", len(chunks))
	}
}

func TestMergeChunks(t *testing.T) {
	chunks := []TextChunk{
		{Content: "宁波弘泰 MSK 美东", AgentName: "宁波弘泰", Region: "美加线", HasPricing: true},
		{Content: "SAV/NWK 2700/3000++", AgentName: "宁波弘泰", Region: "美加线", HasPricing: true},
		{Content: "上海铮航 MSK 美西", AgentName: "上海铮航", Region: "美加线", HasPricing: true},
	}

	merged := MergeChunks(chunks)

	fmt.Printf("合并测试: %d -> %d 个分块\n", len(chunks), len(merged))
	for i, chunk := range merged {
		fmt.Printf("合并后 %d: Agent=%s, 长度=%d\n", i+1, chunk.AgentName, len(chunk.Content))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
