prefix = /opt
srvprefix = /srv
varprefix = /var/opt

srvdir = $(DESTDIR)$(srvprefix)/plantcare
servicedir = $(DESTDIR)/etc/systemd/system

srcdir = .
outdir = .

program = plantcare

files = $(DESTDIR)$(prefix)/bin/$(program)
files += $(patsubst $(srcdir)/service/%,$(DESTDIR)$(prefix)/bin/%,$(wildcard $(srcdir)/service/*.sh))
files += $(patsubst $(srcdir)/service/%,$(servicedir)/%,$(wildcard $(srcdir)/service/*.service $(srcdir)/service/*.path))
files += $(DESTDIR)$(varprefix)/plantcare
files += $(patsubst $(srcdir)/camweb/%,$(srvdir)/camweb/%,$(wildcard $(srcdir)/camweb/*.html))
files += $(patsubst $(srcdir)/web/%,$(srvdir)/web/%,$(wildcard $(srcdir)/web/*.html))
files += $(patsubst $(srcdir)/web/js/%,$(srvdir)/web/js/%,$(wildcard $(srcdir)/web/js/*.js))

all: $(outdir)/$(program)

$(outdir)/$(program): $(wildcard $(srcdir)/*.go)
	GOARM=6 GOARCH=arm GOOS=linux go build -o $@

install: $(files)

$(DESTDIR)$(prefix)/bin/%: $(outdir)/%
	install -DTm755 $< $@

$(DESTDIR)$(varprefix)/plantcare:
	install -d $@

$(DESTDIR)$(prefix)/bin/%.sh: $(srcdir)/service/%.sh
	install -DTm755 $< $@

$(servicedir)/%.service: $(srcdir)/service/%.service
	install -DTm644 $< $@

$(servicedir)/%.path: $(srcdir)/service/%.path
	install -DTm644 $< $@

$(DESTDIR)$(srvprefix)/plantcare/camweb/%.html: $(srcdir)/camweb/%.html
	install -DTm600 $< $@

$(DESTDIR)$(srvprefix)/plantcare/web/%.html: $(srcdir)/web/%.html
	install -DTm600 $< $@

$(DESTDIR)$(srvprefix)/plantcare/web/js/%.js: $(srcdir)/web/js/%.js
	install -DTm600 $< $@

clean:
	rm $(outdir)/$(program)
