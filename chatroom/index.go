package chatroom

import (
	"sync"
)

type BiMap struct {
	mu sync.RWMutex
	kv map[interface{}]interface{}
	vk map[interface{}]interface{}
}

func (bi *BiMap) EveryPair(f func(key, value interface{}) bool, readonly bool) {
	if readonly {
		bi.mu.RLock()
		defer bi.mu.RUnlock()
	} else {
		bi.mu.Lock()
		defer bi.mu.Unlock()
	}
	for k, v := range bi.kv {
		if f(k, v) == false {
			break
		}
	}
}

func (bi *BiMap) AddPair(key, value interface{}) {
	bi.mu.Lock()
	defer bi.mu.Unlock()

	bi.kv[key] = value
	bi.vk[value] = key
}

func (bi *BiMap) RemoveByKey(key interface{}) {
	bi.mu.Lock()
	defer bi.mu.Unlock()

	if value, ok := bi.kv[key]; ok {
		delete(bi.kv, key)
		delete(bi.vk, value)
	}
}

func (bi *BiMap) RemoveByValue(value interface{}) {
	bi.mu.Lock()
	defer bi.mu.Unlock()

	if key, ok := bi.vk[value]; ok {
		delete(bi.vk, value)
		delete(bi.kv, key)
	}
}

func (bi *BiMap) Value(key interface{}) (value interface{}, ok bool) {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	value, ok = bi.kv[key]
	return value, ok
}

func (bi *BiMap) MultiValues(keys []interface{}) (values []interface{}, oks []bool) {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	size := len(keys)
	values = make([]interface{}, size)
	oks = make([]bool, size)

	for i, key := range keys {
		values[i], oks[i] = bi.kv[key]
	}
	return values, oks
}

func (bi *BiMap) AllValues(output interface{}) bool {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	switch output.(type) {
	case *[]interface{}:
		values := output.(*[]interface{})
		if size := len(bi.vk); cap(*values) < size {
			*values = make([]interface{}, 0, size)
		} else {
			*values = (*values)[:0]
		}
		for v, _ := range bi.vk {
			*values = append(*values, v)
		}
		return true
	default:
		return false
	}
}

func (bi *BiMap) Key(value interface{}) (key interface{}, ok bool) {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	key, ok = bi.vk[value]
	return key, ok
}

func (bi *BiMap) MultiKeys(values []interface{}) (keys []interface{}, oks []bool) {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	size := len(keys)
	keys = make([]interface{}, size)
	oks = make([]bool, size)

	for i, value := range values {
		keys[i], oks[i] = bi.vk[value]
	}
	return keys, oks
}

func (bi *BiMap) AllKeys(output interface{}) bool {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	switch output.(type) {
	case *[]interface{}:
		keys := output.(*[]interface{})
		if size := len(bi.kv); cap(*keys) < size {
			*keys = make([]interface{}, 0, size)
		} else {
			*keys = (*keys)[:0]
		}
		for k, _ := range bi.kv {
			*keys = append(*keys, k)
		}
		return true
	default:
		return false
	}
}

func NewBiMap() *BiMap {
	bi := &BiMap{
		kv: make(map[interface{}]interface{}),
		vk: make(map[interface{}]interface{}),
	}
	return bi
}

type set = map[interface{}]struct{}

type BIndex struct {
	mu           sync.RWMutex
	userToTagSet map[interface{}]set // map[key] map[key2]struct{}
	tagToUserSet map[interface{}]set // map[key2] map[key]struct{}
}

func (bi *BIndex) AddUserTag(user interface{}, tags ...interface{}) {
	bi.mu.Lock()
	defer bi.mu.Unlock()

	// 给用户user添加标签tag

	// 正向索引
	tagSet, ok := bi.userToTagSet[user]
	if !ok {
		// user不存在，创建新tagSet并加入
		tagSet = make(set)
		bi.userToTagSet[user] = tagSet
	}

	for _, tag := range tags {
		tagSet[tag] = struct{}{}

		// 反向索引
		userSet, ok := bi.tagToUserSet[tag]
		if !ok {
			// tag不存在，创建新userSet并加入
			userSet = make(set)
			bi.tagToUserSet[tag] = userSet
		}
		userSet[user] = struct{}{}
	}
}

func (bi *BIndex) RemoveUserTag(user interface{}, tags ...interface{}) {
	bi.mu.Lock()
	defer bi.mu.Unlock()

	// 正向索引
	tagSet, ok := bi.userToTagSet[user]
	if !ok {
		// user不存在
		return
	}

	for _, tag := range tags {
		delete(tagSet, tag)
		if len(tagSet) == 0 {
			delete(bi.userToTagSet, user)
		}

		// 反向索引
		userSet, ok := bi.tagToUserSet[tag]
		if !ok {
			// tag不存在，创建新userSet并加入
			return
		}
		delete(userSet, user)
		if len(userSet) == 0 {
			delete(bi.tagToUserSet, tag)
		}
	}
}

func (bi *BIndex) Tags(user interface{}, output interface{}) bool {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	// 正向索引
	tagSet, ok := bi.userToTagSet[user]
	if !ok {
		// user不存在
		return false
	}

	switch output.(type) {
	case *[]interface{}:
		tags := output.(*[]interface{})
		if size := len(tagSet); cap(*tags) < size {
			*tags = make([]interface{}, 0, size)
		} else {
			*tags = (*tags)[:0]
		}
		for tag, _ := range tagSet {
			*tags = append(*tags, tag)
		}
		return true
	default:
		return false
	}
}

func (bi *BIndex) Users(tag interface{}, output interface{}) bool {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	// 反向索引
	userSet, ok := bi.tagToUserSet[tag]
	if !ok {
		// tag不存在
		return false
	}

	switch output.(type) {
	case *[]interface{}:
		users := output.(*[]interface{})
		if size := len(userSet); cap(*users) < size {
			*users = make([]interface{}, 0, size)
		} else {
			*users = (*users)[:0]
		}
		for user, _ := range userSet {
			*users = append(*users, user)
		}
		return true
	default:
		return false
	}
}

func (bi *BIndex) SelectUsers(tags []interface{}, output interface{}) {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	var users *[]interface{}
	switch output.(type) {
	case *[]interface{}:
		users = output.(*[]interface{})
		*users = (*users)[:0]
	default:
		return
	}

	for _, tag := range tags {
		// 反向索引
		userSet, ok := bi.tagToUserSet[tag]
		if ok {
			for user, _ := range userSet {
				*users = append(*users, user)
			}
		}
	}
}

func (bi *BIndex) AllTags(output interface{}) bool {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	switch output.(type) {
	case *[]interface{}:
		tags := output.(*[]interface{})
		if size := len(bi.tagToUserSet); cap(*tags) < size {
			*tags = make([]interface{}, 0, size)
		} else {
			*tags = (*tags)[:0]
		}
		for tag, _ := range bi.tagToUserSet {
			*tags = append(*tags, tag)
		}
		return true
	default:
		return false
	}
}

func (bi *BIndex) AllUsers(output interface{}) bool {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	switch output.(type) {
	case *[]interface{}:
		users := output.(*[]interface{})
		if size := len(bi.userToTagSet); cap(*users) < size {
			*users = make([]interface{}, 0, size)
		} else {
			*users = (*users)[:0]
		}
		for user, _ := range bi.userToTagSet {
			*users = append(*users, user)
		}
		return true
	default:
		return false
	}
}

func (bi *BIndex) RemoveUser(user interface{}) {
	bi.mu.Lock()
	defer bi.mu.Unlock()

	// 正向索引
	tagSet, ok := bi.userToTagSet[user]
	if !ok {
		// user不存在
		return
	}

	for tag, _ := range tagSet {
		userSet := bi.tagToUserSet[tag]
		delete(userSet, user)
		if len(userSet) == 0 {
			delete(bi.tagToUserSet, tag)
		}
	}

	delete(bi.userToTagSet, user)
}

func (bi *BIndex) RemoveTag(tag interface{}) {
	bi.mu.Lock()
	defer bi.mu.Unlock()

	// 反向索引
	userSet, ok := bi.tagToUserSet[tag]
	if !ok {
		// user不存在
		return
	}

	for user, _ := range userSet {
		tagSet := bi.userToTagSet[user]
		delete(tagSet, tag)
		if len(tagSet) == 0 {
			delete(bi.userToTagSet, user)
		}
	}

	delete(bi.tagToUserSet, tag)
}

func NewBIndex() *BIndex {
	bi := &BIndex{
		userToTagSet: make(map[interface{}]set),
		tagToUserSet: make(map[interface{}]set),
	}
	return bi
}

type Index struct {
	mu           sync.RWMutex
	userToTagSet map[interface{}]set // map[key] map[key2]struct{}
	tagToUser    map[interface{}]interface{}
}

func (i *Index) AddUserTag(user interface{}, tags ...interface{}) {
	i.mu.Lock()
	defer i.mu.Unlock()

	// 给用户user添加标签tag

	// 正向索引
	tagSet, ok := i.userToTagSet[user]
	if !ok {
		// user不存在，创建新tagSet并加入
		tagSet = make(set)
		i.userToTagSet[user] = tagSet
	}

	for _, tag := range tags {
		tagSet[tag] = struct{}{}
		if i.tagToUser != nil {
			i.tagToUser[tag] = user
		}
	}
}

func (i *Index) RemoveUserTag(user interface{}, tags ...interface{}) {
	i.mu.Lock()
	defer i.mu.Unlock()

	// 正向索引
	tagSet, ok := i.userToTagSet[user]
	if !ok {
		// user不存在
		return
	}

	for _, tag := range tags {
		delete(tagSet, tag)
		if len(tagSet) == 0 {
			delete(i.userToTagSet, user)
		}
		if i.tagToUser != nil {
			delete(i.tagToUser, tag)
		}
	}
}

func (i *Index) Tags(user interface{}, output interface{}) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// 正向索引
	tagSet, ok := i.userToTagSet[user]
	if !ok {
		// user不存在
		return false
	}

	switch output.(type) {
	case *[]interface{}:
		tags := output.(*[]interface{})
		if size := len(tagSet); cap(*tags) < size {
			*tags = make([]interface{}, 0, size)
		} else {
			*tags = (*tags)[:0]
		}
		for tag, _ := range tagSet {
			*tags = append(*tags, tag)
		}
		return true
	default:
		return false
	}
}

func (i *Index) SelectTags(users []interface{}, output interface{}) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	var tags *[]interface{}
	switch output.(type) {
	case *[]interface{}:
		tags = output.(*[]interface{})
		*tags = (*tags)[:0]
	default:
		return
	}

	for _, user := range users {
		tagSet, ok := i.userToTagSet[user]
		if ok {
			for tag, _ := range tagSet {
				*tags = append(*tags, tag)
			}
		}
	}
}

func (i *Index) AllUsers(output interface{}) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	switch output.(type) {
	case *[]interface{}:
		users := output.(*[]interface{})
		if size := len(i.userToTagSet); cap(*users) < size {
			*users = make([]interface{}, 0, size)
		} else {
			*users = (*users)[:0]
		}
		for user, _ := range i.userToTagSet {
			*users = append(*users, user)
		}
		return true
	default:
		return false
	}
}

func (i *Index) RemoveUser(user interface{}) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.tagToUser != nil {
		// 正向索引
		tagSet, ok := i.userToTagSet[user]
		if !ok {
			// user不存在
			return
		}

		for tag, _ := range tagSet {
			delete(i.tagToUser, tag)
		}
	}
	delete(i.userToTagSet, user)
}

func (i *Index) RemoveTag(tag interface{}) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.tagToUser == nil {
		return
	}

	if user, ok := i.tagToUser[tag]; ok {
		tagSet := i.userToTagSet[user]
		delete(tagSet, tag)
		if len(tagSet) == 0 {
			delete(i.userToTagSet, user)
		}
	}
}

func (i *Index) User(tag interface{}) (interface{}, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.tagToUser == nil {
		return nil, false
	}

	user, ok := i.tagToUser[tag]
	return user, ok
}

func NewIndex(reverse bool) *Index {
	i := &Index{
		userToTagSet: make(map[interface{}]set),
	}
	if reverse {
		i.tagToUser = make(map[interface{}]interface{})
	}
	return i
}
