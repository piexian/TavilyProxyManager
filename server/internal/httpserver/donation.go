package httpserver

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"tavily-proxy/server/internal/models"
	"tavily-proxy/server/internal/services"
	"tavily-proxy/server/internal/util"
)

type donationItem struct {
	KeyMasked string `json:"key_masked"`
	Alias     string `json:"alias"`
	Status    string `json:"status"` // created, duplicated, invalid, invalid_key
}

// pendingDonation 是已通过格式、去重与上游校验、待入库的 Key。
type pendingDonation struct {
	idx   int // 对应 donationBatch.items 的下标
	key   string
	usage int
	limit *int
}

// donationBatch 把一次捐赠拆成「校验（只读、走上游）」与「入库（本地写）」两阶段：
// 校验期间不落库，避免中途失败或客户端断连留下半成品。
type donationBatch struct {
	alias   string
	items   []donationItem
	pending []pendingDonation
}

// validateDonations 逐个校验捐赠 Key（格式、请求内/库内去重、上游 /usage），不写库。
func validateDonations(ctx context.Context, raws []string, alias string, keys *services.KeyService, proxy *services.TavilyProxy) (*donationBatch, error) {
	b := &donationBatch{alias: alias, items: make([]donationItem, 0, len(raws))}
	seen := make(map[string]struct{}, len(raws))
	add := func(key, status string) int {
		b.items = append(b.items, donationItem{KeyMasked: util.MaskAPIKey(key), Alias: alias, Status: status})
		return len(b.items) - 1
	}

	for _, raw := range raws {
		key := strings.TrimSpace(raw)
		if !donateKeyPattern.MatchString(key) {
			add(key, "invalid")
			continue
		}
		if _, dup := seen[key]; dup {
			add(key, "duplicated")
			continue
		}
		seen[key] = struct{}{}

		existing, err := keys.QueryByKey(ctx, key)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			add(key, "duplicated")
			continue
		}
		// 先校验再入库，临时故障不会阻止重试。
		usage, limit, err := proxy.GetUsage(ctx, key)
		if err != nil {
			add(key, "invalid_key")
			continue
		}
		b.pending = append(b.pending, pendingDonation{idx: add(key, "created"), key: key, usage: usage, limit: limit})
	}
	return b, nil
}

// persist 把校验通过的 Key 以「已激活 + 已归属」的形态逐个单条 INSERT 入库，返回本次新建的 Key ID。
// 并发下被抢先入库的 Key 标记为 duplicated。出错时同样返回已新建的 ID，便于调用方回滚。
func (b *donationBatch) persist(ctx context.Context, keys *services.KeyService, accessKeyID uint) ([]uint, error) {
	created := make([]uint, 0, len(b.pending))
	for _, p := range b.pending {
		record, isNew, err := keys.DonateOwnedKey(ctx, p.key, b.alias, accessKeyID, p.usage, p.limit)
		if err != nil {
			return created, err
		}
		if !isNew {
			b.items[p.idx].Status = "duplicated"
			continue
		}
		created = append(created, record.ID)
	}
	return created, nil
}

// persistForNewDonor 先创建捐赠者访问密钥，再把 Key 带归属入库，不存在「已激活但无主」的 Key。
// 失败时回滚（删除已入库的 Key 与访问密钥），释放 alias，捐赠者可原样重试；
// 没有任何 Key 实际入库时同样不保留访问密钥。accessKeys 为 nil 时退化为无主入库。
func (b *donationBatch) persistForNewDonor(ctx context.Context, keys *services.KeyService, accessKeys *services.AccessKeyService) (*models.AccessKey, error) {
	if len(b.pending) == 0 {
		return nil, nil
	}

	var ak *models.AccessKey
	var accessKeyID uint
	if accessKeys != nil {
		var err error
		if ak, err = accessKeys.Create(ctx, b.alias); err != nil {
			return nil, err
		}
		accessKeyID = ak.ID
	}

	createdIDs, err := b.persist(ctx, keys, accessKeyID)
	if err == nil && len(createdIDs) > 0 {
		return ak, nil
	}
	for _, id := range createdIDs {
		_ = keys.Delete(ctx, id)
	}
	if ak != nil {
		_ = accessKeys.Delete(ctx, ak.ID)
	}
	return nil, err
}

func (b *donationBatch) response() gin.H {
	var created, duplicated, invalid int
	for _, it := range b.items {
		switch it.Status {
		case "created":
			created++
		case "duplicated":
			duplicated++
		default:
			invalid++
		}
	}
	return gin.H{
		"total":      len(b.items),
		"created":    created,
		"duplicated": duplicated,
		"invalid":    invalid,
		"activated":  created,
		"items":      b.items,
	}
}
