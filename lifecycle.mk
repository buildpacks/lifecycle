define build_lifecycle
	@echo "> Building lifecycle for $(TARGET)..."
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/lifecycle -a ./cmd/lifecycle
	@echo "> Creating lifecycle symlinks for $(1)..."
	ln -sf lifecycle $(OUT_DIR)/detector
	ln -sf lifecycle $(OUT_DIR)/analyzer
	ln -sf lifecycle $(OUT_DIR)/restorer
	ln -sf lifecycle $(OUT_DIR)/builder
	ln -sf lifecycle $(OUT_DIR)/exporter
	ln -sf lifecycle $(OUT_DIR)/rebaser
	ln -sf lifecycle $(OUT_DIR)/creator
	ln -sf lifecycle $(OUT_DIR)/extender
endef
