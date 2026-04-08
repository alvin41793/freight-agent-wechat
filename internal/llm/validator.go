package llm

// ValidateRates 验证运价记录的必要字段。
// 返回：完整的运价列表、不完整记录的索引到缺失字段列表的映射。
func ValidateRates(rates []FreightRate) (valid []FreightRate, incomplete map[int][]string) {
	incomplete = make(map[int][]string)

	for i, r := range rates {
		var missing []string

		if r.POL == "" {
			missing = append(missing, "POL（起运港）")
		}
		if r.POD == "" {
			missing = append(missing, "POD（目的港）")
		}
		if r.F20GP == "" && r.F40GP == "" && r.F40HC == "" {
			missing = append(missing, "运价（F20GP / F40GP / F40HC 至少填写一个）")
		}
		if r.ValidityStartTime == "" {
			missing = append(missing, "ValidityStartTime（有效期开始日期）")
		}
		if r.ValidityEndTime == "" {
			missing = append(missing, "ValidityEndTime（有效期结束日期）")
		}

		if len(missing) == 0 {
			valid = append(valid, r)
		} else {
			incomplete[i] = missing
		}
	}

	return valid, incomplete
}
