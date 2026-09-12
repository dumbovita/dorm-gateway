# dorm-gateway

GSB yurt WiFi giriş ekranlarına (captive portal) kimlik doğrulaması yapan ve isteğe bağlı Cloudflare WARP entegrasyonu sunan hafif ve hızlı bir CLI aracı.

[![macOS](https://img.shields.io/badge/macOS-000000?style=flat-square&logo=apple&logoColor=white)](#kurulum)
[![Linux](https://img.shields.io/badge/Linux-FCC624?style=flat-square&logo=linux&logoColor=black)](#kurulum)
[![License: MIT](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)

[English](README.md) | **Türkçe**

---

## Kurulum

### Hazır Derlenmiş İkili Dosyalar
İşletim sisteminize ve mimarinize uygun ikili dosyayı [Releases](https://github.com/dumbovita/dorm-gateway/releases) sayfasından indirin, çalıştırma yetkisi verin ve `PATH` dizininize taşıyın:

```bash
chmod +x dorm-gateway
sudo mv dorm-gateway /usr/local/bin/
```

> **macOS Notu:** Gatekeeper engeliyle karşılaşırsanız (*"Apple kötü amaçlı yazılım..."*), **Sistem Ayarları > Gizlilik ve Güvenlik** bölümünden ("Yine de İzin Ver") onaylayabilir veya karantina özniteliğini kaldırabilirsiniz:
> ```bash
> xattr -d com.apple.quarantine /usr/local/bin/dorm-gateway
> ```

### Kaynak Koddan Derleme
[Go 1.22+](https://go.dev/) gerektirir:

```bash
git clone https://github.com/dumbovita/dorm-gateway.git
cd dorm-gateway
make build
sudo mv bin/dorm-gateway /usr/local/bin/
```

---

## Yapılandırma

Giriş bilgilerinizi kısıtlı dosya izinleriyle (`0600`) bir kez kaydedin:

```bash
# Güvenli etkileşimli giriş (şifre ekranda gizlenir)
dorm-gateway config set
```

Veya ortam değişkenleriyle yapılandırın:

```bash
export DORM_GATEWAY_USERNAME="12345678901"
export DORM_GATEWAY_PASSWORD="sifreniz"
```

Mevcut ayarları görüntülemek veya yapılandırma dosyasının konumunu öğrenmek için:
```bash
dorm-gateway config show
dorm-gateway config path
```

---

## Kullanım

### Kimlik Doğrulama
Captive portal giriş ekranında oturum açın:
```bash
dorm-gateway auth
```

### Tek Adımda Bağlanma (`up`)
Portalda oturum açın ve Cloudflare WARP tünelini tek adımda bağlayın:
```bash
dorm-gateway up
```

### Durum Kontrolü
Portal kimlik doğrulamasını, internet bağlantısını ve WARP durumunu görüntüleyin:
```bash
dorm-gateway status
```

### Sistem Tanılama
DNS çözümlemesini, portal erişilebilirliğini, dosya izinlerini ve WARP durumunu denetleyin:
```bash
dorm-gateway doctor
```

---

## Cloudflare WARP (İsteğe Bağlı)

`dorm-gateway`, captive portal oturumu açıldıktan sonra şifreli bir gizlilik tüneli sağlamak için resmi Cloudflare WARP (`warp-cli`) istemcisiyle entegre çalışır:

```bash
# WARP bağlantısını başlat
dorm-gateway warp up

# WARP bağlantısını kes
dorm-gateway warp down

# WARP tünel durumunu incele
dorm-gateway warp status
```

Sisteminizde `warp-cli` kurulu değilse macOS ([1.1.1.1](https://1.1.1.1)) veya Linux ([pkg.cloudflareclient.com](https://pkg.cloudflareclient.com/)) için resmi paketi kurabilirsiniz.

---

## Sorun Giderme

- **Portal zaman aşımı veya ağ yoğunluğu:** Giriş portalları yoğun kullanım saatlerinde yüksek trafikle karşılaşabilir. `dorm-gateway auth`, üstel geri çekilme ve rastgele gecikme (jitter) uygulayarak otomatik olarak yeniden dener.
- **WARP bağlı ancak internet trafiği yok:** Yurt ağınız UDP trafiğini engelliyor olabilir. MASQUE protokolüne geçiş yapabilirsiniz:
  ```bash
  dorm-gateway warp up --protocol masque
  ```
- **Sistem kontrolleri:** Yerel DNS çözümlemesini, internet erişimini ve WARP arka plan hizmetinin (daemon) durumunu doğrulamak için `dorm-gateway doctor` komutunu çalıştırın.

---

## Sorumluluk Reddi

Bu yazılım yalnızca eğitim ve bilgilendirme amacıyla "olduğu gibi" (as-is) sağlanmış olup herhangi bir garanti içermez. Yazılımın kullanımı tamamen kullanıcının kendi sorumluluğundadır. Kullanıcılar yürürlükteki tüm yasalara, kurumsal düzenlemelere, ağ politikalarına ve hizmet koşullarına uymaktan bizzat sorumludur. İlgili yasaların izin verdiği ölçüde; proje geliştiricileri ve katkıda bulunanlar, bu aracın kullanımından veya kötüye kullanımından kaynaklanabilecek hesap askıya alınmaları, ağ erişim kısıtlamaları, hizmet kesintileri, veri kaybı, disiplin cezaları veya doğrudan ya da dolaylı herhangi bir sonuçtan dolayı hiçbir sorumluluk kabul etmez.

---

## Emeği Geçenler

- **İlham & Konsept:** Deniz Egemen Emare ([@denizZz009](https://github.com/denizZz009))

---

## Lisans

[MIT Lisansı](LICENSE) kapsamında dağıtılmaktadır.

---

⭐ Bu araç işinize yaradıysa GitHub üzerinden bir yıldız bırakarak destek olabilirsiniz!
