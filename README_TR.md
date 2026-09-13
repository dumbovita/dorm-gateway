# dorm-gateway

GSB yurt Wi-Fi ağlarında giriş ekranına (captive portal) hızlı ve otomatik kimlik doğrulama sağlayan, isteğe bağlı Cloudflare WARP entegrasyonlu hafif ve pratik bir CLI aracı.

[![macOS](https://img.shields.io/badge/macOS-000000?style=flat-square&logo=apple&logoColor=white)](#kurulum)
[![Linux](https://img.shields.io/badge/Linux-FCC624?style=flat-square&logo=linux&logoColor=black)](#kurulum)
[![License: MIT](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)

[English](README.md) | **Türkçe**

---

## Kurulum

### Hazır Derlenmiş Sürümler
İşletim sisteminize ve mimarinize uygun çalıştırılabilir dosyayı [Releases](https://github.com/dumbovita/dorm-gateway/releases) sayfasından indirin, çalıştırma izni verin ve `PATH` dizininize taşıyın:

```bash
chmod +x dorm-gateway
sudo mv dorm-gateway /usr/local/bin/
```

> **macOS Notu:** Gatekeeper uyarısıyla karşılaşırsanız (*"Apple kötü amaçlı yazılım olup olmadığını denetleyemiyor..."* veya *"Geliştirici doğrulanamadı"*), **Sistem Ayarları > Gizlilik ve Güvenlik** menüsünden ("Yine de Aç / İzin Ver") onaylayabilir veya terminalden karantina özniteliğini kaldırabilirsiniz:
> ```bash
> xattr -d com.apple.quarantine /usr/local/bin/dorm-gateway
> ```

### Kaynak Koddan Derleme
[Go 1.22+](https://go.dev/) veya üzeri gereklidir:

```bash
git clone https://github.com/dumbovita/dorm-gateway.git
cd dorm-gateway
make build
sudo mv bin/dorm-gateway /usr/local/bin/
```

---

## Yapılandırma

Giriş bilgilerinizi yalnızca sizin erişebileceğiniz güvenli dosya izinleriyle (`0600`) tek seferde kaydedin:

```bash
# Güvenli ve interaktif komut istemi (şifre ekranda gizlenir)
dorm-gateway config set
```

Alternatif olarak ortam değişkenlerini (environment variables) kullanabilirsiniz:

```bash
export DORM_GATEWAY_USERNAME="12345678901"
export DORM_GATEWAY_PASSWORD="sifreniz"
```

Mevcut ayarları görüntülemek veya yapılandırma dosyasının yolunu görmek için:
```bash
dorm-gateway config show
dorm-gateway config path
```

---

## Kullanım

### Kimlik Doğrulama
Yurt giriş ekranında (captive portal) oturum açın:
```bash
dorm-gateway auth
```

### Tek Komutla Bağlanma (`up`)
Portalda oturum açın ve Cloudflare WARP tünelini tek adımda başlatın:
```bash
dorm-gateway up
```

### Durum Kontrolü
Portal oturumunu, internet erişimini ve WARP durumunu kontrol edin:
```bash
dorm-gateway status
```

### Sistem Tanılama
DNS çözümlemesini, portal erişilebilirliğini, yapılandırma izinlerini ve WARP durumunu test edin:
```bash
dorm-gateway doctor
```

---

## Cloudflare WARP (İsteğe Bağlı)

`dorm-gateway`, captive portal oturumu açıldıktan sonra internet trafiğinizi şifreli bir gizlilik tüneline almak için resmi Cloudflare WARP (`warp-cli`) istemcisiyle entegre çalışır:

```bash
# WARP bağlantısını başlat
dorm-gateway warp up

# WARP bağlantısını kes
dorm-gateway warp down

# WARP tünel durumunu kontrol et
dorm-gateway warp status
```

Sisteminizde `warp-cli` kurulu değilse macOS ([1.1.1.1](https://1.1.1.1)) veya Linux ([pkg.cloudflareclient.com](https://pkg.cloudflareclient.com/)) için resmi paketi kurabilirsiniz.

---

## Sorun Giderme

- **Portal zaman aşımı veya ağ yoğunluğu:** Yurt giriş portalları yoğun kullanım saatlerinde yüksek trafikle karşılaşabilir veya yanıt vermeyebilir. `dorm-gateway auth`, kademeli artan bekleme süreleriyle (exponential backoff ve jitter) bağlantıyı otomatik olarak yeniden dener.
- **WARP bağlı ancak internet trafiği yok:** Yurt ağınız UDP trafiğini engelliyor olabilir. Bu durumda MASQUE protokolüne geçiş yapabilirsiniz:
  ```bash
  dorm-gateway warp up --protocol masque
  ```
- **Sistem kontrolleri:** Yerel DNS çözümlemesini, internet erişimini ve WARP arka plan servisinin (daemon) durumunu doğrulamak için `dorm-gateway doctor` komutunu çalıştırın.

---

## Sorumluluk Reddi

Bu yazılım yalnızca eğitim ve bilgilendirme amacıyla "olduğu gibi" (as-is) sağlanmış olup hiçbir garanti içermez. Yazılımın kullanımı tamamen kullanıcının kendi sorumluluğundadır. Kullanıcılar yürürlükteki tüm yasalara, kurumsal düzenlemelere, ağ politikalarına ve hizmet koşullarına uymakla bizzat yükümlüdür. Yürürlükteki yasaların izin verdiği azami ölçüde; proje geliştiricileri ve katkıda bulunanlar, bu aracın kullanımından veya kötüye kullanımından kaynaklanabilecek hesap askıya alınmaları, ağ erişim kısıtlamaları, hizmet kesintileri, veri kaybı, disiplin yaptırımları veya doğrudan ya da dolaylı herhangi bir sonuçtan dolayı hiçbir sorumluluk kabul etmez.

---

## Katkıda Bulunanlar

- **İlham & Konsept:** Deniz Egemen Emare ([@denizZz009](https://github.com/denizZz009))

---

## Lisans

Bu proje [MIT Lisansı](LICENSE) kapsamında dağıtılmaktadır.

---

⭐ Bu araç işinize yaradıysa GitHub üzerinden bir yıldız bırakarak destek olabilirsiniz!
