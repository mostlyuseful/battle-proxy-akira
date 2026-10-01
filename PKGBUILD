# Maintainer: Battle Proxy Akira contributors
pkgname=battle-proxy-akira-git
pkgver=0.0.0.r0.g0000000
pkgrel=1
pkgdesc='Small OpenAI-compatible LLM proxy with provider routing and fallback'
arch=('x86_64' 'aarch64')
url='https://github.com/mostlyuseful/battle-proxy-akira'
license=('unknown')
makedepends=('go')
depends=('systemd')
provides=('battle-proxy-akira')
conflicts=('battle-proxy-akira')
backup=('etc/battle-proxy-akira/config.json')
source=("battle-proxy-akira::git+https://github.com/mostlyuseful/battle-proxy-akira.git#branch=main")
sha256sums=('SKIP')

pkgver() {
  cd "$srcdir/battle-proxy-akira"
  printf '0.0.0.r%s.g%s' "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)"
}

build() {
  cd "$srcdir/battle-proxy-akira"
  CGO_ENABLED=0 go build -trimpath -o "$srcdir/llm-proxy" ./cmd/llm-proxy
}

check() {
  cd "$srcdir/battle-proxy-akira"
  CGO_ENABLED=0 go test ./...
}

package() {
  cd "$srcdir/battle-proxy-akira"

  install -Dm755 "$srcdir/llm-proxy" "$pkgdir/usr/bin/battle-proxy-akira"
  install -Dm640 config.example.json "$pkgdir/etc/battle-proxy-akira/config.json"
  install -Dm644 packaging/systemd/battle-proxy-akira.service \
    "$pkgdir/usr/lib/systemd/system/battle-proxy-akira.service"
  install -Dm644 packaging/sysusers/battle-proxy-akira.conf \
    "$pkgdir/usr/lib/sysusers.d/battle-proxy-akira.conf"
  install -Dm644 packaging/tmpfiles.d/battle-proxy-akira.conf \
    "$pkgdir/usr/lib/tmpfiles.d/battle-proxy-akira.conf"
  install -Dm644 README.md \
    "$pkgdir/usr/share/doc/battle-proxy-akira/README.md"
}
