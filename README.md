# 开发者指南
## 基础设施
### ES

Client:
```go
// NewElasticSearchClient https://www.elastic.co/docs/reference/elasticsearch/clients/go/examples
func NewElasticSearchClient(lc fx.Lifecycle, conf *conf.Bootstrap, logger *zap.Logger) (*elasticsearch.TypedClient, error) {
	cfg := conf.Search.ElasticSearch
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Elasticsearch 通常是高频内部调用，默认的 MaxIdleConnsPerHost（默认为 2）可能太小了
	// 如果并发请求很多，这会导致连接频繁创建和销毁，造成大量 TIME_WAIT
	// transport.MaxIdleConnsPerHost = 20

	if cfg.Tls.Enable {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.Tls.InsecureSkipVerify}
		if cfg.Tls.CaPem != "" {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM([]byte(cfg.Tls.CaPem)) {
				transport.TLSClientConfig.RootCAs = pool
			}
		}
	}

	esCfg := elasticsearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
		Logger:    &elastictransport.ColorLogger{Output: os.Stdout},
		Transport: transport,
	}

	es, err := elasticsearch.NewTypedClient(esCfg)
	if err != nil {
		logger.Error("failed to initialize elasticsearch client", zap.Error(err))
		return nil, err
	}

	logger.Info("Elasticsearch client initialized", zap.Strings("addresses", cfg.Addresses))

	return es, nil
}
```

Check:
```go
// CheckElasticSearch 检查ES连通性
func (d *Data) CheckElasticSearch(ctx context.Context) error {
	if d.es == nil {
		return fmt.Errorf("elasticsearch client not initialized")
	}
	// 调用 Ping 方法
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := d.es.Ping().Do(ctx)
	if err != nil {
		return fmt.Errorf("elastic search ping failed: %w", err)
	}

	// 如果没有 err 且响应为 true，说明服务在线
	if res {
		fmt.Println("elastic search is alive!")
	}
	return nil
}
```
