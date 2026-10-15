package repairform

import (
	"strings"

	"github.com/imega/mygarage/store"
)

type Group struct {
	Name  string
	Items []store.WorkType
}

func Groups(types []store.WorkType) []Group {
	var list []Group
	index := map[string]int{}
	for _, item := range types {
		name := strings.TrimSpace(item.Category)
		if name == "" {
			continue
		}
		i, ok := index[name]
		if !ok {
			i = len(list)
			index[name] = i
			list = append(list, Group{Name: name})
		}
		list[i].Items = append(list[i].Items, item)
	}

	return list
}
