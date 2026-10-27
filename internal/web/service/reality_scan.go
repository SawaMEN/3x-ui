package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/util/common"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

const (
	realityScanTimeout     = 10 * time.Second
	realityDiscoverTimeout = 4 * time.Second
	realityScanConcurrency = 32
	realityDiscoverMaxIPs  = 256
	realityScanMaxTotal    = 1024
)

var legacyRealityScanCandidates = []string{
	"www.cloudflare.com:443",
	"www.microsoft.com:443",
	"www.amazon.com:443",
	"aws.amazon.com:443",
	"www.samsung.com:443",
	"www.nvidia.com:443",
	"www.amd.com:443",
	"www.intel.com:443",
	"www.sony.com:443",
	"dl.google.com:443",
}

// Previously shipped 60-entry default, retained for exact-match upgrades.
var previousRealityScanCandidates = []string{
	"www.cloudflare.com:443",
	"www.microsoft.com:443",
	"www.amazon.com:443",
	"aws.amazon.com:443",
	"www.samsung.com:443",
	"www.nvidia.com:443",
	"www.amd.com:443",
	"www.intel.com:443",
	"www.sony.com:443",
	"dl.google.com:443",
	"www.google.com:443",
	"www.bing.com:443",
	"www.apple.com:443",
	"www.mozilla.org:443",
	"www.firefox.com:443",
	"www.wikipedia.org:443",
	"www.github.com:443",
	"github.com:443",
	"www.gitlab.com:443",
	"about.gitlab.com:443",
	"www.adobe.com:443",
	"www.autodesk.com:443",
	"www.cisco.com:443",
	"www.dell.com:443",
	"www.hp.com:443",
	"www.lenovo.com:443",
	"www.asus.com:443",
	"www.acer.com:443",
	"www.logitech.com:443",
	"www.canon.com:443",
	"www.nikon.com:443",
	"www.panasonic.com:443",
	"www.philips.com:443",
	"www.bosch.com:443",
	"www.siemens.com:443",
	"www.ibm.com:443",
	"www.oracle.com:443",
	"www.sap.com:443",
	"www.salesforce.com:443",
	"www.atlassian.com:443",
	"www.dropbox.com:443",
	"www.zoom.com:443",
	"www.spotify.com:443",
	"www.yahoo.com:443",
	"www.duckduckgo.com:443",
	"www.debian.org:443",
	"www.ubuntu.com:443",
	"www.kernel.org:443",
	"www.python.org:443",
	"www.rust-lang.org:443",
	"yandex.ru:443",
	"ya.ru:443",
	"vk.com:443",
	"ok.ru:443",
	"mail.ru:443",
	"www.ozon.ru:443",
	"www.wildberries.ru:443",
	"www.avito.ru:443",
	"www.sberbank.ru:443",
	"www.gosuslugi.ru:443",
}

// Candidate seeds, not a promise of reachability or protocol compatibility.
// Each search probes certificates and TLS capabilities before offering them.
var previous160RealityScanCandidates = slices.Concat(previousRealityScanCandidates, []string{
	"ozon.ru:443",
	"hh.ru:443",
	"www.hh.ru:443",
	"superjob.ru:443",
	"rabota.ru:443",
	"zarplata.ru:443",
	"market.yandex.ru:443",
	"dzen.ru:443",
	"rutube.ru:443",
	"kinopoisk.ru:443",
	"music.yandex.ru:443",
	"disk.yandex.ru:443",
	"cloud.yandex.ru:443",
	"2gis.ru:443",
	"www.tutu.ru:443",
	"www.aviasales.ru:443",
	"travel.yandex.ru:443",
	"www.cian.ru:443",
	"www.domclick.ru:443",
	"www.sravni.ru:443",
	"www.banki.ru:443",
	"www.tbank.ru:443",
	"www.vtb.ru:443",
	"alfabank.ru:443",
	"www.gazprombank.ru:443",
	"www.mts.ru:443",
	"megafon.ru:443",
	"beeline.ru:443",
	"t2.ru:443",
	"www.rostelecom.ru:443",
	"www.rbc.ru:443",
	"www.ria.ru:443",
	"tass.ru:443",
	"lenta.ru:443",
	"www.gazeta.ru:443",
	"www.kommersant.ru:443",
	"www.vedomosti.ru:443",
	"www.interfax.ru:443",
	"habr.com:443",
	"www.dns-shop.ru:443",
	"www.mvideo.ru:443",
	"www.eldorado.ru:443",
	"www.citilink.ru:443",
	"www.lamoda.ru:443",
	"www.sportmaster.ru:443",
	"www.detmir.ru:443",
	"www.vseinstrumenti.ru:443",
	"www.auchan.ru:443",
	"www.perekrestok.ru:443",
	"www.magnit.ru:443",
	"www.lenta.com:443",
	"www.sdek.ru:443",
	"www.pochta.ru:443",
	"www.rzd.ru:443",
	"www.aeroflot.ru:443",
	"www.pobeda.aero:443",
	"www.kaspersky.ru:443",
	"www.drweb.ru:443",
	"www.ivi.ru:443",
	"www.okko.tv:443",
	"www.netflix.com:443",
	"www.youtube.com:443",
	"www.reddit.com:443",
	"www.twitch.tv:443",
	"www.pinterest.com:443",
	"www.instagram.com:443",
	"www.facebook.com:443",
	"www.linkedin.com:443",
	"www.tiktok.com:443",
	"www.paypal.com:443",
	"www.ebay.com:443",
	"www.aliexpress.com:443",
	"www.booking.com:443",
	"www.airbnb.com:443",
	"www.tripadvisor.com:443",
	"www.imdb.com:443",
	"www.bbc.com:443",
	"www.cnn.com:443",
	"www.nytimes.com:443",
	"www.theguardian.com:443",
	"www.bloomberg.com:443",
	"www.reuters.com:443",
	"www.notion.so:443",
	"www.figma.com:443",
	"www.canva.com:443",
	"www.npmjs.com:443",
	"pypi.org:443",
	"store.steampowered.com:443",
	"www.epicgames.com:443",
	"www.roblox.com:443",
	"discord.com:443",
	"telegram.org:443",
	"www.whatsapp.com:443",
	"www.signal.org:443",
	"proton.me:443",
	"www.fastly.com:443",
	"www.digitalocean.com:443",
	"www.akamai.com:443",
	"www.ovhcloud.com:443",
	"www.hetzner.com:443",
})

// 800 candidate endpoints: original defaults, Russian services and additional
// domains from Majestic Million (https://majestic.com/reports/majestic-million).
// Seeds are unverified until probed from the panel; inclusion is not a claim
// of REALITY compatibility, client reachability or whitelist availability.
var defaultRealityScanCandidates = slices.Concat(previous160RealityScanCandidates, []string{
	"rambler.ru:443",
	"sber.ru:443",
	"sberbank.com:443",
	"samokat.ru:443",
	"vkusvill.ru:443",
	"petrovich.ru:443",
	"lemanapro.ru:443",
	"hoff.ru:443",
	"askona.ru:443",
	"onlinetrade.ru:443",
	"holodilnik.ru:443",
	"tehnopark.ru:443",
	"re-store.ru:443",
	"goldapple.ru:443",
	"letual.ru:443",
	"rivegauche.ru:443",
	"apteka.ru:443",
	"eapteka.ru:443",
	"zdravcity.ru:443",
	"uteka.ru:443",
	"mos.ru:443",
	"nalog.gov.ru:443",
	"cbr.ru:443",
	"government.ru:443",
	"kremlin.ru:443",
	"consultant.ru:443",
	"garant.ru:443",
	"pravo.gov.ru:443",
	"sudrf.ru:443",
	"kad.arbitr.ru:443",
	"zakupki.gov.ru:443",
	"bus.gov.ru:443",
	"rostrud.gov.ru:443",
	"trudvsem.ru:443",
	"profi.ru:443",
	"youdo.com:443",
	"travelata.ru:443",
	"level.travel:443",
	"onetwotrip.com:443",
	"ostrovok.ru:443",
	"sutochno.ru:443",
	"bronevik.com:443",
	"tvil.ru:443",
	"s7.ru:443",
	"utair.ru:443",
	"uralairlines.ru:443",
	"nordwindairlines.ru:443",
	"rossiya-airlines.com:443",
	"cdek.ru:443",
	"boxberry.ru:443",
	"dpd.ru:443",
	"ponyexpress.ru:443",
	"dellin.ru:443",
	"pecom.ru:443",
	"realty.yandex.ru:443",
	"auto.ru:443",
	"drom.ru:443",
	"drive2.ru:443",
	"quto.ru:443",
	"autonews.ru:443",
	"zr.ru:443",
	"kolesa.ru:443",
	"wink.ru:443",
	"start.ru:443",
	"more.tv:443",
	"premier.one:443",
	"kion.ru:443",
	"smotrim.ru:443",
	"ntv.ru:443",
	"1tv.ru:443",
	"ren.tv:443",
	"tvzvezda.ru:443",
	"matchtv.ru:443",
	"sportbox.ru:443",
	"championat.com:443",
	"sports.ru:443",
	"sport-express.ru:443",
	"kp.ru:443",
	"aif.ru:443",
	"mk.ru:443",
	"rg.ru:443",
	"iz.ru:443",
	"fontanka.ru:443",
	"e1.ru:443",
	"ngs.ru:443",
	"74.ru:443",
	"59.ru:443",
	"72.ru:443",
	"63.ru:443",
	"76.ru:443",
	"29.ru:443",
	"93.ru:443",
	"ufa1.ru:443",
	"v1.ru:443",
	"161.ru:443",
	"meduza.io:443",
	"t-j.ru:443",
	"vc.ru:443",
	"dtf.ru:443",
	"pikabu.ru:443",
	"liveinternet.ru:443",
	"otzovik.com:443",
	"irecommend.ru:443",
	"vk.ru:443",
	"vkvideo.ru:443",
	"vkplay.ru:443",
	"vkcloud.ru:443",
	"cloud.ru:443",
	"selectel.ru:443",
	"timeweb.com:443",
	"timeweb.cloud:443",
	"beget.com:443",
	"reg.ru:443",
	"nic.ru:443",
	"firstvds.ru:443",
	"adminvps.ru:443",
	"sprinthost.ru:443",
	"hostkey.ru:443",
	"spaceweb.ru:443",
	"mchost.ru:443",
	"netangels.ru:443",
	"fornex.com:443",
	"itsoft.ru:443",
	"ddos-guard.net:443",
	"securitycode.ru:443",
	"astralinux.ru:443",
	"basealt.ru:443",
	"red-soft.ru:443",
	"myoffice.ru:443",
	"r7-office.ru:443",
	"onlyoffice.com:443",
	"1c.ru:443",
	"1c-bitrix.ru:443",
	"bitrix24.ru:443",
	"kontur.ru:443",
	"sbis.ru:443",
	"tensor.ru:443",
	"moedelo.org:443",
	"tochka.com:443",
	"modulbank.ru:443",
	"psbank.ru:443",
	"sovcombank.ru:443",
	"otpbank.ru:443",
	"raiffeisen.ru:443",
	"uralsib.ru:443",
	"rncb.ru:443",
	"akbars.ru:443",
	"mkb.ru:443",
	"domrfbank.ru:443",
	"finam.ru:443",
	"bcs.ru:443",
	"moex.com:443",
	"spbexchange.ru:443",
	"megamarket.ru:443",
	"aliexpress.ru:443",
	"kazanexpress.ru:443",
	"uzum.uz:443",
	"ozon.kz:443",
	"ozon.by:443",
	"wildberries.by:443",
	"wildberries.kz:443",
	"ozon.com:443",
	"ozon.travel:443",
	"litres.ru:443",
	"labirint.ru:443",
	"chitai-gorod.ru:443",
	"bookvoed.ru:443",
	"book24.ru:443",
	"eksmo.ru:443",
	"ast.ru:443",
	"mann-ivanov-ferber.ru:443",
	"netology.ru:443",
	"skillbox.ru:443",
	"skillfactory.ru:443",
	"practicum.yandex.ru:443",
	"stepik.org:443",
	"foxford.ru:443",
	"uchi.ru:443",
	"resh.edu.ru:443",
	"skyeng.ru:443",
	"skysmart.ru:443",
	"puzzle-english.com:443",
	"duolingo.com:443",
	"lingualeo.com:443",
	"postnauka.ru:443",
	"arzamas.academy:443",
	"nplus1.ru:443",
	"elementy.ru:443",
	"naked-science.ru:443",
	"techinsider.ru:443",
	"ixbt.com:443",
	"3dnews.ru:443",
	"overclockers.ru:443",
	"ferra.ru:443",
	"hi-tech.mail.ru:443",
	"trashbox.ru:443",
	"4pda.to:443",
	"ixbt.games:443",
	"stopgame.ru:443",
	"playground.ru:443",
	"igromania.ru:443",
	"ag.ru:443",
	"cyber.sports.ru:443",
	"cybersport.ru:443",
	"lesta.ru:443",
	"tanki.su:443",
	"mir-korabley.ru:443",
	"warthunder.ru:443",
	"twitter.com:443",
	"play.google.com:443",
	"youtu.be:443",
	"apps.apple.com:443",
	"x.com:443",
	"en.wikipedia.org:443",
	"wa.me:443",
	"maps.google.com:443",
	"wordpress.org:443",
	"docs.google.com:443",
	"policies.google.com:443",
	"vimeo.com:443",
	"nginx.org:443",
	"f5.com:443",
	"nginx.com:443",
	"maps.app.goo.gl:443",
	"api.whatsapp.com:443",
	"drive.google.com:443",
	"t.me:443",
	"europa.eu:443",
	"support.google.com:443",
	"wordpress.com:443",
	"bit.ly:443",
	"player.vimeo.com:443",
	"blogspot.com:443",
	"apache.org:443",
	"support.microsoft.com:443",
	"forms.gle:443",
	"nih.gov:443",
	"qq.com:443",
	"itunes.apple.com:443",
	"httpd.apache.org:443",
	"open.spotify.com:443",
	"ec.europa.eu:443",
	"tumblr.com:443",
	"archive.org:443",
	"gravatar.com:443",
	"godaddy.com:443",
	"creativecommons.org:443",
	"sites.google.com:443",
	"office.com:443",
	"w3.org:443",
	"flickr.com:443",
	"doi.org:443",
	"support.mozilla.org:443",
	"baidu.com:443",
	"medium.com:443",
	"youtube-nocookie.com:443",
	"soundcloud.com:443",
	"accounts.google.com:443",
	"forbes.com:443",
	"web.archive.org:443",
	"sciencedirect.com:443",
	"live.com:443",
	"zoom.us:443",
	"www.ncbi.nlm.nih.gov:443",
	"googleblog.com:443",
	"sourceforge.net:443",
	"namecheap.com:443",
	"cpanel.net:443",
	"who.int:443",
	"shopify.com:443",
	"wixsite.com:443",
	"opera.com:443",
	"t.co:443",
	"workspaceupdates.googleblog.com:443",
	"wikimedia.org:443",
	"researchgate.net:443",
	"nature.com:443",
	"tinyurl.com:443",
	"mit.edu:443",
	"launchpad.net:443",
	"bbc.co.uk:443",
	"php.net:443",
	"springer.com:443",
	"discord.gg:443",
	"pubmed.ncbi.nlm.nih.gov:443",
	"wiley.com:443",
	"harvard.edu:443",
	"weibo.com:443",
	"gnu.org:443",
	"cookiedatabase.org:443",
	"cdc.gov:443",
	"un.org:443",
	"bugs.launchpad.net:443",
	"mp.weixin.qq.com:443",
	"linktr.ee:443",
	"issuu.com:443",
	"plesk.com:443",
	"openai.com:443",
	"weebly.com:443",
	"stanford.edu:443",
	"beian.gov.cn:443",
	"m.facebook.com:443",
	"www.gov.uk:443",
	"outlook.com:443",
	"g.page:443",
	"stripe.com:443",
	"wpa.qq.com:443",
	"washingtonpost.com:443",
	"telegram.me:443",
	"calendly.com:443",
	"nasa.gov:443",
	"arxiv.org:443",
	"wsj.com:443",
	"commons.wikimedia.org:443",
	"msn.com:443",
	"unsplash.com:443",
	"trustpilot.com:443",
	"statista.com:443",
	"ietf.org:443",
	"businessinsider.com:443",
	"amzn.to:443",
	"bsky.app:443",
	"line.me:443",
	"list-manage.com:443",
	"share.google:443",
	"podcasts.apple.com:443",
	"substack.com:443",
	"mysql.com:443",
	"calendar.google.com:443",
	"sharepoint.com:443",
	"cnbc.com:443",
	"eventbrite.com:443",
	"oup.com:443",
	"openstreetmap.org:443",
	"gouv.fr:443",
	"wired.com:443",
	"etsy.com:443",
	"time.com:443",
	"npr.org:443",
	"stackoverflow.com:443",
	"tandfonline.com:443",
	"chatgpt.com:443",
	"fr.wikipedia.org:443",
	"behance.net:443",
	"unesco.org:443",
	"techcrunch.com:443",
	"iso.org:443",
	"slideshare.net:443",
	"gc.ca:443",
	"cloud.microsoft:443",
	"hubspot.com:443",
	"validator.w3.org:443",
	"mdpi.com:443",
	"sagepub.com:443",
	"ft.com:443",
	"loc.gov:443",
	"outlook.live.com:443",
	"outlook.office.com:443",
	"aol.com:443",
	"go.com:443",
	"google.de:443",
	"rfc-editor.org:443",
	"fda.gov:443",
	"web.facebook.com:443",
	"berkeley.edu:443",
	"cornell.edu:443",
	"zendesk.com:443",
	"squarespace.com:443",
	"ca.gov:443",
	"telegraph.co.uk:443",
	"bilibili.com:443",
	"jotform.com:443",
	"usatoday.com:443",
	"de.wikipedia.org:443",
	"miit.gov.cn:443",
	"baike.baidu.com:443",
	"bandcamp.com:443",
	"g.co:443",
	"wpguardian.com:443",
	"wpguardian.io:443",
	"cnet.com:443",
	"dailymail.co.uk:443",
	"scholar.google.com:443",
	"aliyun.com:443",
	"redhat.com:443",
	"surveymonkey.com:443",
	"britannica.com:443",
	"cbsnews.com:443",
	"amazon.co.uk:443",
	"blogger.com:443",
	"sina.com.cn:443",
	"mckinsey.com:443",
	"cambridge.org:443",
	"prnewswire.com:443",
	"allaboutcookies.org:443",
	"wix.com:443",
	"nist.gov:443",
	"ted.com:443",
	"pixabay.com:443",
	"ieee.org:443",
	"free.fr:443",
	"mailchimp.com:443",
	"independent.co.uk:443",
	"hbr.org:443",
	"theverge.com:443",
	"readthedocs.io:443",
	"frontiersin.org:443",
	"163.com:443",
	"steampowered.com:443",
	"worldbank.org:443",
	"dailymotion.com:443",
	"princeton.edu:443",
	"epa.gov:443",
	"dx.doi.org:443",
	"myshopify.com:443",
	"web.whatsapp.com:443",
	"docker.com:443",
	"yelp.com:443",
	"huawei.com:443",
	"usda.gov:443",
	"taobao.com:443",
	"mailchi.mp:443",
	"myspace.com:443",
	"oecd.org:443",
	"weforum.org:443",
	"latimes.com:443",
	"whitehouse.gov:443",
	"imgur.com:443",
	"apnews.com:443",
	"mps.gov.cn:443",
	"goodreads.com:443",
	"washington.edu:443",
	"nationalgeographic.com:443",
	"finance.yahoo.com:443",
	"investopedia.com:443",
	"deloitte.com:443",
	"safety.google:443",
	"youku.com:443",
	"scribd.com:443",
	"microsoftonline.com:443",
	"academia.edu:443",
	"fandom.com:443",
	"noaa.gov:443",
	"fb.com:443",
	"mayoclinic.org:443",
	"state.gov:443",
	"indiatimes.com:443",
	"plos.org:443",
	"theatlantic.com:443",
	"ox.ac.uk:443",
	"pexels.com:443",
	"nbcnews.com:443",
	"themeforest.net:443",
	"marriott.com:443",
	"huffpost.com:443",
	"beian.mps.gov.cn:443",
	"tencent.com:443",
	"apa.org:443",
	"columbia.edu:443",
	"arcgis.com:443",
	"xing.com:443",
	"freepik.com:443",
	"amazon.de:443",
	"sohu.com:443",
	"translate.google.com:443",
	"indeed.com:443",
	"ftc.gov:443",
	"ico.org.uk:443",
	"livejournal.com:443",
	"v.qq.com:443",
	"yale.edu:443",
	"odoo.com:443",
	"webmd.com:443",
	"patreon.com:443",
	"theconversation.com:443",
	"pbs.org:443",
	"kickstarter.com:443",
	"canada.ca:443",
	"umich.edu:443",
	"biomedcentral.com:443",
	"fb.me:443",
	"android.com:443",
	"economist.com:443",
	"change.org:443",
	"businesswire.com:443",
	"healthline.com:443",
	"quora.com:443",
	"postgresql.org:443",
	"eepurl.com:443",
	"eff.org:443",
	"fortune.com:443",
	"bmj.com:443",
	"dribbble.com:443",
	"google.co.uk:443",
	"ovh.com:443",
	"googleapis.com:443",
	"es.wikipedia.org:443",
	"enable-javascript.com:443",
	"admin.ch:443",
	"typeform.com:443",
	"bbb.org:443",
	"gartner.com:443",
	"abcnews.go.com:443",
	"upenn.edu:443",
	"alibaba.com:443",
	"psu.edu:443",
	"bund.de:443",
	"pewresearch.org:443",
	"threads.net:443",
	"jimdo.com:443",
	"cbc.ca:443",
	"fao.org:443",
	"psychologytoday.com:443",
	"skype.com:443",
	"foxnews.com:443",
	"zhihu.com:443",
	"letsencrypt.org:443",
	"jstor.org:443",
	"moodle.org:443",
	"chat.whatsapp.com:443",
	"cam.ac.uk:443",
	"usnews.com:443",
	"uci.edu:443",
	"akismet.com:443",
	"wpastra.com:443",
	"zdnet.com:443",
	"deviantart.com:443",
	"gofundme.com:443",
	"abc.net.au:443",
	"sciencedaily.com:443",
	"nypost.com:443",
	"news.google.com:443",
	"elsevier.com:443",
	"coursera.org:443",
	"umn.edu:443",
	"acm.org:443",
	"music.apple.com:443",
	"scientificamerican.com:443",
	"fb.watch:443",
	"newyorker.com:443",
	"elegantthemes.com:443",
	"xinhuanet.com:443",
	"thelancet.com:443",
	"threads.com:443",
	"irs.gov:443",
	"windows.net:443",
	"blog.google:443",
	"vice.com:443",
	"arstechnica.com:443",
	"newsweek.com:443",
	"oreilly.com:443",
	"anthropic.com:443",
	"lnkd.in:443",
	"nps.gov:443",
	"dw.com:443",
	"orcid.org:443",
	"git-scm.com:443",
	"jamanetwork.com:443",
	"cmu.edu:443",
	"themeisle.com:443",
	"m.me:443",
	"uchicago.edu:443",
	"mitre.org:443",
	"wa.link:443",
	"fastcompany.com:443",
	"slack.com:443",
	"about.com:443",
	"pnas.org:443",
	"acs.org:443",
	"bls.gov:443",
	"stackexchange.com:443",
	"wisc.edu:443",
	"pan.baidu.com:443",
	"ucla.edu:443",
	"gizmodo.com:443",
	"wiktionary.org:443",
	"typepad.com:443",
	"tripod.com:443",
	"naver.com:443",
	"hilton.com:443",
	"jd.com:443",
	"globenewswire.com:443",
	"huggingface.co:443",
	"www.nhs.uk:443",
	"usgs.gov:443",
	"people.com.cn:443",
	"si.edu:443",
	"getbootstrap.com:443",
	"elpais.com:443",
	"openssl.org:443",
	"techtarget.com:443",
	"itu.int:443",
	"science.org:443",
	"aljazeera.com:443",
	"jhu.edu:443",
	"iana.org:443",
	"euronews.com:443",
	"unicef.org:443",
	"ssrn.com:443",
	"sec.gov:443",
	"wikihow.com:443",
	"ed.gov:443",
	"sciencemag.org:443",
	"hhs.gov:443",
	"aboutcookies.org:443",
	"lin.ee:443",
	"magnific.com:443",
	"utexas.edu:443",
	"wanwang.aliyun.com:443",
	"walmart.com:443",
	"engadget.com:443",
	"mashable.com:443",
	"ru.wikipedia.org:443",
	"justice.gov:443",
	"meetup.com:443",
	"giphy.com:443",
	"w3schools.com:443",
	"matterport.com:443",
	"lemonde.fr:443",
	"merriam-webster.com:443",
	"service.gov.uk:443",
	"house.gov:443",
	"storage.googleapis.com:443",
	"uiuc.edu:443",
	"www.gov.cn:443",
	"automattic.com:443",
	"teamviewer.com:443",
	"pwc.com:443",
	"ikea.com:443",
	"fedoraproject.org:443",
	"spiegel.de:443",
	"freebsd.org:443",
	"docs.oracle.com:443",
	"clevelandclinic.org:443",
	"visualstudio.com:443",
	"nodejs.org:443",
	"web.dev:443",
	"census.gov:443",
	"laravel.com:443",
	"intuit.com:443",
})

// DefaultRealityScanCandidatesCSV is the shipped default for the
// realityScanCandidates setting (comma-separated host:port list).
var DefaultRealityScanCandidatesCSV = strings.Join(defaultRealityScanCandidates, ",")

// Expand only exact previously shipped defaults. Custom lists remain authoritative.
func expandedRealityScanCandidates(csv string) string {
	tokens := parseRealityScanCandidateCSV(csv)
	if slices.Equal(tokens, legacyRealityScanCandidates) || slices.Equal(tokens, previousRealityScanCandidates) || slices.Equal(tokens, previous160RealityScanCandidates) {
		return DefaultRealityScanCandidatesCSV
	}
	return csv
}

type RealityScanResult struct {
	Target   string `json:"target" example:"www.cloudflare.com:443"`
	Host     string `json:"host" example:"www.cloudflare.com"`
	IP       string `json:"ip" example:"104.16.124.96"`
	Port     int    `json:"port" example:"443"`
	Feasible bool   `json:"feasible" example:"true"`
	// PrivateTarget marks a target that resolves to a loopback/private/link-local
	// address: blocked before the probe unless the caller opted in, then flagged.
	PrivateTarget bool   `json:"privateTarget" example:"false"`
	TLS13         bool   `json:"tls13" example:"true"`
	TLSVersion    string `json:"tlsVersion" example:"1.3"`
	H2            bool   `json:"h2" example:"true"`
	ALPN          string `json:"alpn" example:"h2"`
	X25519        bool   `json:"x25519" example:"true"`
	CurveID       string `json:"curveID" example:"X25519"`
	CertValid     bool   `json:"certValid" example:"true"`
	// CertChainValid ignores the name: a trusted chain presented for other names
	// still has serverNames the panel can offer instead of the failing SNI.
	CertChainValid bool `json:"certChainValid" example:"true"`
	// CertChainBytes is the sum of DER lengths of the presented peer chain.
	// xray-core ML-DSA-65 REALITY needs >= 3500 bytes (constant lives in xray-core).
	CertChainBytes int      `json:"certChainBytes" example:"3427"`
	CertSubject    string   `json:"certSubject" example:"cloudflare.com"`
	CertIssuer     string   `json:"certIssuer" example:"Google Trust Services"`
	NotAfter       string   `json:"notAfter" example:"2026-08-01T00:00:00Z"`
	ServerNames    []string `json:"serverNames"`
	LatencyMs      int      `json:"latencyMs" example:"180"`
	Reason         string   `json:"reason" example:""`
}

type realityProbeTask struct {
	dialHost string
	port     int
	sni      string
	timeout  time.Duration
	bulk     bool
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "1.3"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS10:
		return "1.0"
	default:
		return "unknown"
	}
}

func realityCurveName(id tls.CurveID) string {
	switch id {
	case tls.X25519:
		return "X25519"
	case tls.X25519MLKEM768:
		return "X25519MLKEM768"
	case tls.CurveP256:
		return "P-256"
	case tls.CurveP384:
		return "P-384"
	case tls.CurveP521:
		return "P-521"
	case 0:
		return ""
	default:
		return fmt.Sprintf("0x%04x", uint16(id))
	}
}

func filterUsableSANs(dnsNames []string) []string {
	out := make([]string, 0, len(dnsNames))
	for _, n := range dnsNames {
		n = strings.TrimSpace(n)
		if n == "" || strings.HasPrefix(n, "*.") {
			continue
		}
		out = append(out, n)
	}
	return out
}

func firstUsableName(leaf *x509.Certificate) string {
	cn := strings.TrimSpace(leaf.Subject.CommonName)
	if cn != "" && !strings.HasPrefix(cn, "*.") {
		return cn
	}
	for _, n := range leaf.DNSNames {
		n = strings.TrimSpace(n)
		if n != "" && !strings.HasPrefix(n, "*.") {
			return n
		}
	}
	return ""
}

func leafVerifies(leaf *x509.Certificate, opts x509.VerifyOptions) bool {
	_, err := leaf.Verify(opts)
	return err == nil
}

func splitRealityTarget(target string) (string, int, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", 0, common.NewError("target is required")
	}
	host, portStr := target, "443"
	if h, p, err := net.SplitHostPort(target); err == nil {
		host, portStr = h, p
	}
	host, err := netsafe.NormalizeHost(host)
	if err != nil {
		return "", 0, common.NewError("invalid target host: ", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, common.NewError("invalid target port")
	}
	return host, port, nil
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func enumerateCIDR(cidr string, max int) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return nil, err
	}
	ips := make([]string, 0, max)
	for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		ips = append(ips, ip.String())
		if len(ips) >= max {
			break
		}
	}
	return ips, nil
}

func (s *ServerService) probeRealityAddr(dialHost string, port int, sni string, timeout time.Duration, xver int, allowPrivate bool) *RealityScanResult {
	addr := net.JoinHostPort(dialHost, strconv.Itoa(port))
	res := &RealityScanResult{Port: port}
	if net.ParseIP(dialHost) != nil {
		res.IP = dialHost
	}
	// Target stays the dialed address (it is what the inbound dials); Host is
	// the SNI the handshake sent, which may differ for a fronting proxy.
	res.Host = dialHost
	res.Target = addr
	if sni != "" {
		res.Host = sni
	}

	ctx, cancel := context.WithTimeout(netsafe.ContextWithAllowPrivate(context.Background(), allowPrivate), timeout)
	defer cancel()

	start := time.Now()
	conn, err := netsafe.SSRFGuardedDialContext(ctx, "tcp", addr)
	if err != nil {
		res.PrivateTarget = errors.Is(err, netsafe.ErrPrivateAddressBlocked)
		res.Reason = "connection failed: " + err.Error()
		return res
	}
	defer conn.Close()
	if remote, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		res.PrivateTarget = netsafe.IsBlockedIP(remote.IP)
		// The opt-in bypasses the SSRF guard, so leave an audit trail of it.
		if res.PrivateTarget && allowPrivate {
			logger.Infof("reality scan reached private target %s (%s) with the operator opt-in", addr, remote.IP)
		}
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))

	// A REALITY inbound with xver>=1 fronts a target that speaks the PROXY
	// protocol (e.g. an Nginx listener with `proxy_protocol`), so the probe
	// must lead with a PROXY header or the target resets the connection and
	// the scan reports a spurious handshake failure (#6082).
	if xver >= 1 {
		if err := writeProxyProtocolHeader(conn, xver); err != nil {
			res.Reason = "proxy protocol write failed: " + err.Error()
			return res
		}
	}

	cfg := &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
		CurvePreferences:   []tls.CurveID{tls.X25519, tls.X25519MLKEM768},
		MinVersion:         tls.VersionTLS12,
	}
	tlsConn := tls.Client(conn, cfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		res.Reason = "TLS handshake failed: " + err.Error()
		return res
	}
	res.LatencyMs = int(time.Since(start).Milliseconds())

	st := tlsConn.ConnectionState()
	res.TLS13 = st.Version == tls.VersionTLS13
	res.TLSVersion = tlsVersionName(st.Version)
	res.ALPN = st.NegotiatedProtocol
	res.H2 = st.NegotiatedProtocol == "h2"
	res.CurveID = realityCurveName(st.CurveID)
	res.X25519 = st.CurveID == tls.X25519 || st.CurveID == tls.X25519MLKEM768

	verifyHost := sni
	if len(st.PeerCertificates) > 0 {
		leaf := st.PeerCertificates[0]
		for _, cert := range st.PeerCertificates {
			res.CertChainBytes += len(cert.Raw)
		}
		res.CertSubject = leaf.Subject.CommonName
		if res.CertSubject == "" && len(leaf.DNSNames) > 0 {
			res.CertSubject = leaf.DNSNames[0]
		}
		if len(leaf.Issuer.Organization) > 0 {
			res.CertIssuer = leaf.Issuer.Organization[0]
		} else {
			res.CertIssuer = leaf.Issuer.CommonName
		}
		res.NotAfter = leaf.NotAfter.UTC().Format(time.RFC3339)
		res.ServerNames = filterUsableSANs(leaf.DNSNames)

		if sni == "" {
			if discovered := firstUsableName(leaf); discovered != "" {
				res.Host = discovered
				res.Target = net.JoinHostPort(discovered, strconv.Itoa(port))
				verifyHost = discovered
			}
		}

		if verifyHost != "" {
			opts := x509.VerifyOptions{Intermediates: x509.NewCertPool()}
			for _, c := range st.PeerCertificates[1:] {
				opts.Intermediates.AddCert(c)
			}
			// The chain is checked without the name first: a publicly trusted
			// certificate for other names still carries usable serverNames.
			res.CertChainValid = leafVerifies(leaf, opts)
			opts.DNSName = verifyHost
			if leafVerifies(leaf, opts) {
				res.CertValid = true
			} else {
				_, verr := leaf.Verify(opts)
				res.Reason = "certificate not trusted: " + verr.Error()
			}
		} else {
			res.Reason = "no usable domain in certificate"
		}
	} else {
		res.Reason = "no certificate presented"
	}

	res.Feasible = res.TLS13 && res.H2 && res.X25519 && res.CertValid
	if !res.Feasible && res.Reason == "" {
		switch {
		case !res.TLS13:
			res.Reason = "server does not negotiate TLS 1.3"
		case !res.H2:
			res.Reason = "server does not negotiate HTTP/2 (h2)"
		case !res.X25519:
			res.Reason = "server did not use X25519 key exchange"
		}
	}
	return res
}

// ScanRealityTarget probes one operator-supplied target. An empty sni falls back
// to the target host; allowPrivate lifts the SSRF guard for this probe only.
func (s *ServerService) ScanRealityTarget(target string, sni string, xver int, allowPrivate bool) (*RealityScanResult, error) {
	host, port, err := splitRealityTarget(target)
	if err != nil {
		return nil, err
	}
	sni = strings.TrimSpace(sni)
	if sni == "" {
		sni = host
	} else if sni, err = netsafe.NormalizeHost(sni); err != nil {
		return nil, common.NewError("invalid SNI: ", err)
	}
	return s.probeRealityAddr(host, port, sni, realityScanTimeout, xver, allowPrivate), nil
}

func parseRealityScanCandidateCSV(csv string) []string {
	var tokens []string
	for raw := range strings.SplitSeq(csv, ",") {
		if t := strings.TrimSpace(raw); t != "" {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// realityScanCandidateTokens returns the operator-configured candidate list,
// falling back to the shipped defaults when the setting is empty or unreadable.
func (s *ServerService) realityScanCandidateTokens() []string {
	csv, err := s.settingService.GetRealityScanCandidates()
	if err != nil {
		logger.Warning("reality scan: reading candidates setting failed:", err)
	} else if tokens := parseRealityScanCandidateCSV(csv); len(tokens) > 0 {
		return tokens
	}
	return append([]string(nil), defaultRealityScanCandidates...)
}

func (s *ServerService) ScanRealityTargets(targetsCSV string) ([]*RealityScanResult, error) {
	tokens := parseRealityScanCandidateCSV(targetsCSV)
	if len(tokens) == 0 {
		tokens = s.realityScanCandidateTokens()
	}

	var tasks []realityProbeTask
	var invalid []*RealityScanResult
	for _, token := range tokens {
		if len(tasks) >= realityScanMaxTotal {
			break
		}
		if strings.Contains(token, "/") {
			ips, err := enumerateCIDR(token, realityDiscoverMaxIPs)
			if err != nil {
				invalid = append(invalid, &RealityScanResult{Target: token, Reason: "invalid CIDR: " + err.Error()})
				continue
			}
			for _, ip := range ips {
				if len(tasks) >= realityScanMaxTotal {
					break
				}
				tasks = append(tasks, realityProbeTask{dialHost: ip, port: 443, timeout: realityDiscoverTimeout, bulk: true})
			}
			continue
		}
		host, port, err := splitRealityTarget(token)
		if err != nil {
			invalid = append(invalid, &RealityScanResult{Target: token, Reason: err.Error()})
			continue
		}
		if net.ParseIP(host) != nil {
			tasks = append(tasks, realityProbeTask{dialHost: host, port: port, timeout: realityDiscoverTimeout})
		} else {
			tasks = append(tasks, realityProbeTask{dialHost: host, port: port, sni: host, timeout: realityScanTimeout})
		}
	}

	probed := make([]*RealityScanResult, len(tasks))
	sem := make(chan struct{}, realityScanConcurrency)
	var wg sync.WaitGroup
	for i, task := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, tk realityProbeTask) {
			defer wg.Done()
			defer func() { <-sem }()
			// The bulk/CIDR scanner never reaches private ranges: the opt-in
			// there would turn it into an internal network scanner.
			r := s.probeRealityAddr(tk.dialHost, tk.port, tk.sni, tk.timeout, 0, false)
			if tk.bulk && r.TLSVersion == "" {
				return
			}
			probed[idx] = r
		}(i, task)
	}
	wg.Wait()

	results := dedupRealityResults(append(probed, invalid...))
	sortRealityResults(results)
	return results, nil
}

func dedupRealityResults(results []*RealityScanResult) []*RealityScanResult {
	best := make(map[string]*RealityScanResult)
	order := make([]string, 0, len(results))
	for _, r := range results {
		if r == nil {
			continue
		}
		if ex, ok := best[r.Target]; !ok {
			best[r.Target] = r
			order = append(order, r.Target)
		} else if betterRealityResult(r, ex) {
			best[r.Target] = r
		}
	}
	out := make([]*RealityScanResult, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	return out
}

func betterRealityResult(a, b *RealityScanResult) bool {
	if a.Feasible != b.Feasible {
		return a.Feasible
	}
	return a.LatencyMs > 0 && (b.LatencyMs == 0 || a.LatencyMs < b.LatencyMs)
}

func sortRealityResults(results []*RealityScanResult) {
	slices.SortStableFunc(results, func(a, b *RealityScanResult) int {
		if a.Feasible != b.Feasible {
			if a.Feasible {
				return -1
			}
			return 1
		}
		return a.LatencyMs - b.LatencyMs
	})
}

// writeProxyProtocolHeader emits a PROXY protocol header describing the local
// connection so a target that requires it (Nginx `proxy_protocol`, matching a
// REALITY inbound's xver) accepts the probe instead of resetting it. xver 1
// sends the human-readable v1 header; xver 2 sends the binary v2 header. The
// addresses come from the already-dialed connection, so they are always a
// consistent, real (src, dst) pair.
func writeProxyProtocolHeader(conn net.Conn, xver int) error {
	local, lok := conn.LocalAddr().(*net.TCPAddr)
	remote, rok := conn.RemoteAddr().(*net.TCPAddr)
	if !lok || !rok {
		return fmt.Errorf("connection has no TCP addresses")
	}
	if xver >= 2 {
		return writeProxyProtocolV2(conn, local, remote)
	}
	return writeProxyProtocolV1(conn, local, remote)
}

func writeProxyProtocolV1(conn net.Conn, local, remote *net.TCPAddr) error {
	fam := "TCP4"
	if local.IP.To4() == nil || remote.IP.To4() == nil {
		fam = "TCP6"
	}
	header := fmt.Sprintf("PROXY %s %s %s %d %d\r\n", fam, local.IP.String(), remote.IP.String(), local.Port, remote.Port)
	_, err := conn.Write([]byte(header))
	return err
}

func writeProxyProtocolV2(conn net.Conn, local, remote *net.TCPAddr) error {
	buf := []byte{0x0D, 0x0A, 0x0D, 0x0A, 0x00, 0x0D, 0x0A, 0x51, 0x55, 0x49, 0x54, 0x0A}
	buf = append(buf, 0x21)

	src4, dst4 := local.IP.To4(), remote.IP.To4()
	if src4 != nil && dst4 != nil {
		buf = append(buf, 0x11)
		buf = append(buf, 0x00, 12)
		buf = append(buf, src4...)
		buf = append(buf, dst4...)
		buf = append(buf, byte(local.Port>>8), byte(local.Port))
		buf = append(buf, byte(remote.Port>>8), byte(remote.Port))
	} else {
		buf = append(buf, 0x21)
		buf = append(buf, 0x00, 36)
		buf = append(buf, local.IP.To16()...)
		buf = append(buf, remote.IP.To16()...)
		buf = append(buf, byte(local.Port>>8), byte(local.Port))
		buf = append(buf, byte(remote.Port>>8), byte(remote.Port))
	}
	_, err := conn.Write(buf)
	return err
}
