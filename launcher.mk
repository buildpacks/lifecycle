define build_launcher
	@echo "> Building launcher for $(TARGET)..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/launcher -a ./cmd/launcher
	test $$(du -m $(OUT_DIR)/launcher | cut -f 1) -le 4
endef