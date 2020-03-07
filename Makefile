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
files += $(DESTDIR)$(varprefix)/plantcare
files += $(DESTDIR)$(varprefix)/plantcare/pics
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

$(DESTDIR)$(varprefix)/plantcare/pics:
	install -d $@

$(DESTDIR)$(prefix)/bin/%.sh: $(srcdir)/service/%.sh
	install -DTm755 $< $@

$(DESTDIR)$(srvprefix)/plantcare/web/%.html: $(srcdir)/web/%.html
	install -DTm600 $< $@

$(DESTDIR)$(srvprefix)/plantcare/web/js/%.js: $(srcdir)/web/js/%.js
	install -DTm600 $< $@

clean:
	rm $(outdir)/$(program)
