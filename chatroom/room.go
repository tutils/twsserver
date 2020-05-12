package chatroom

import (
	"fmt"
	"github.com/tutils/twsserver"
	"sync/atomic"
)

var (
	cliId int64 = 0
)

func genid() int64 {
	id := atomic.AddInt64(&cliId, 1)
	if id == 0 {
		return atomic.AddInt64(&cliId, 1)
	}
	return id
}

type Room struct {
	clients *BiMap  // id <-> Client
	where   *BIndex // Client -> channel set, channel -> Client set
	who     *Index  // uid -> Client set

	distribute Distribute
}

func (r *Room) AddClient(cli twsserver.Client) int64 {
	id := genid()
	r.clients.AddPair(id, cli)
	return id
}

func (r *Room) Login(cli twsserver.Client, uid int64) {
	r.who.AddUserTag(uid, cli)

	if r.distribute != nil {
		if id, ok := r.clients.Key(cli); ok {
			r.distribute.Register(fmt.Sprintf(RegClientKeyFmt, id))
			r.distribute.Register(fmt.Sprintf(RegUserKeyFmt, uid))
		}
	}
}

func (r *Room) ClientsOfUser(uid int64) (twsserver.ClientGroup, bool) {
	var out []interface{}
	if ok := r.who.Tags(uid, &out); !ok {
		return nil, false
	}
	return twsserver.NewClientGroup(out), true
}

func (r *Room) ClientsOfUsers(uids []int64) twsserver.ClientGroup {
	uids_ := make([]interface{}, len(uids))
	for i, uid := range uids {
		uids_[i] = uid
	}
	var out []interface{}
	r.who.SelectTags(uids_, &out)
	return twsserver.NewClientGroup(out)
}

func (r *Room) RemoveClient(cli twsserver.Client) {
	if r.distribute != nil {
		if id, ok := r.clients.Key(cli); ok {
			r.distribute.Unregister(fmt.Sprintf(RegClientKeyFmt, id))
		}
		if uid, ok := r.who.User(cli); ok {
			r.distribute.Unregister(fmt.Sprintf(RegUserKeyFmt, uid))
		}
	}

	r.clients.RemoveByValue(cli)
	r.where.RemoveUser(cli)
	r.who.RemoveTag(cli)
}

func (r *Room) ClientEnterChannel(cli twsserver.Client, chs ...string) {
	chs_ := make([]interface{}, 0, len(chs))
	for _, ch := range chs {
		if len(ch) != 0 {
			chs_ = append(chs_, ch)
		}
	}
	r.where.AddUserTag(cli, chs_...)
}

func (r *Room) ClientExitChannel(cli twsserver.Client, chs ...string) {
	chs_ := make([]interface{}, len(chs))
	for i, ch := range chs {
		chs_[i] = ch
	}
	r.where.RemoveUserTag(cli, chs_...)
}

func (r *Room) ClientsInChannel(ch string) (twsserver.ClientGroup, bool) {
	var out []interface{}
	if ok := r.where.Users(ch, &out); !ok {
		return nil, false
	}
	return twsserver.NewClientGroup(out), true
}

func (r *Room) ClientsInChannels(chs []string) twsserver.ClientGroup {
	chs_ := make([]interface{}, len(chs))
	for i, ch := range chs {
		chs_[i] = ch
	}
	var out []interface{}
	r.where.SelectUsers(chs_, &out)
	return twsserver.NewClientGroup(out)
}

func (r *Room) Client(id int64) (twsserver.Client, bool) {
	if cli_, ok := r.clients.Value(id); ok {
		return cli_.(twsserver.Client), true
	} else {
		return nil, false
	}
}

func (r *Room) Clients(ids []int64) twsserver.ClientGroup {
	ids_ := make([]interface{}, len(ids))
	for i, id := range ids {
		ids_[i] = id
	}
	clis_, oks := r.clients.MultiValues(ids_)
	clis := make([]interface{}, 0, len(ids))
	for i, ok := range oks {
		if ok {
			clis = append(clis, clis_[i])
		}
	}
	return twsserver.NewClientGroup(clis)
}

func (r *Room) ClientId(cli twsserver.Client) (int64, bool) {
	if id_, ok := r.clients.Key(cli); ok {
		return id_.(int64), true
	} else {
		return 0, false
	}
}

func (r *Room) User(cli twsserver.Client) (int64, bool) {
	if uid_, ok := r.who.User(cli); ok {
		return uid_.(int64), true
	} else {
		return 0, false
	}
}

func NewRoom(distribute Distribute) *Room {
	r := &Room{
		clients: NewBiMap(),
		where:   NewBIndex(),
		who:     NewIndex(true),

		distribute: distribute,
	}
	return r
}
