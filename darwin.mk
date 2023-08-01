define build_darwin_lifecycle
	@echo "> Building lifecycle for $(TARGET)..."
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/lifecycle -a ./cmd/lifecycle
	@echo "> Creating lifecycle symlinks for $(1)..."
	ln -sf lifecycle $(OUT_DIR)/detector
	ln -sf lifecycle $(OUT_DIR)/analyzer
	ln -sf lifecycle $(OUT_DIR)/restorer
	ln -sf lifecycle $(OUT_DIR)/builder
	ln -sf lifecycle $(OUT_DIR)/exporter
	ln -sf lifecycle $(OUT_DIR)/rebaser
endef

define build_darwin_launcher
	@echo "> Building launcher for $(TARGET)..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/launcher -a ./cmd/launcher
	test $$(du -m $(OUT_DIR)/launcher | cut -f 1) -le 4
endef