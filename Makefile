.PHONY: lexicons
lexicons: lexicons/app/bsky lexicons/chat/bsky lexicons/com/atproto lexicons/tools/ozone
	@echo -n

lexicons/app/bsky: lexicons/app
	@ln -s ../../../atproto/$@ ./$@
lexicons/chat/bsky: lexicons/chat
	@ln -s ../../../atproto/$@ ./$@
lexicons/com/atproto: lexicons/com
	@ln -s ../../../atproto/$@ ./$@
lexicons/tools/ozone: lexicons/tools
	@ln -s ../../../atproto/$@ ./$@
lexicons/app lexicons/chat lexicons/com lexicons/tools:
	mkdir -p $@
