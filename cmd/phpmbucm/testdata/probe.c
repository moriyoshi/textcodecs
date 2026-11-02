/* Compile against a configured php-src checkout; include the actual mbfl
 * conversion functions and tables so mappings computed by code are included. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "filters/mbfilter_cjk.c"
#include "filters/mbfilter_singlebyte.c"

/* The probe provides ample output space; allocation is never expected. */
void *ZEND_FASTCALL _erealloc(void *ptr, size_t size) { (void)ptr; (void)size; abort(); }
void mb_illegal_output(uint32_t cp, mb_from_wchar_fn fn, mb_convert_buf *buf) {
    (void)cp; (void)fn; buf->errors++;
}

typedef struct { const char *name; mb_to_wchar_fn decode; mb_from_wchar_fn encode; int euc; } codec;
#define SB(n, id) {n, mb_##id##_to_wchar, mb_wchar_to_##id, 3}
static const codec codecs[] = {
    SB("ASCII", ascii), SB("ISO-8859-1", 8859_1), SB("ISO-8859-2", 8859_2),
    SB("ISO-8859-3", 8859_3), SB("ISO-8859-4", 8859_4), SB("ISO-8859-5", 8859_5),
    SB("ISO-8859-6", 8859_6), SB("ISO-8859-7", 8859_7), SB("ISO-8859-8", 8859_8),
    SB("ISO-8859-9", 8859_9), SB("ISO-8859-10", 8859_10), SB("ISO-8859-13", 8859_13),
    SB("ISO-8859-14", 8859_14), SB("ISO-8859-15", 8859_15), SB("ISO-8859-16", 8859_16),
    SB("Windows-1251", cp1251), SB("Windows-1252", cp1252), SB("Windows-1254", cp1254),
    SB("CP866", cp866), SB("CP850", cp850), SB("KOI8-R", koi8r), SB("KOI8-U", koi8u),
    SB("ArmSCII-8", armscii8),
    {"SJIS", mb_sjis_to_wchar, mb_wchar_to_sjis, 0},
    {"SJIS-2004", mb_sjis2004_to_wchar, mb_wchar_to_sjis2004, 0},
    {"SJIS-mac", mb_sjismac_to_wchar, mb_wchar_to_sjismac, 0},
    {"SJIS-Mobile-DOCOMO", mb_sjis_docomo_to_wchar, mb_wchar_to_sjis_docomo, 0},
    {"SJIS-Mobile-KDDI", mb_sjis_kddi_to_wchar, mb_wchar_to_sjis_kddi, 0},
    {"SJIS-Mobile-SOFTBANK", mb_sjis_sb_to_wchar, mb_wchar_to_sjis_sb, 0},
    {"CP932", mb_cp932_to_wchar, mb_wchar_to_cp932, 0},
    {"SJIS-win", mb_cp932_to_wchar, mb_wchar_to_sjiswin, 0},
    {"EUC-JP", mb_eucjp_to_wchar, mb_wchar_to_eucjp, 1},
    {"eucJP-win", mb_eucjpwin_to_wchar, mb_wchar_to_eucjpwin, 1},
    {"CP51932", mb_cp51932_to_wchar, mb_wchar_to_cp51932, 1},
    {"EUC-JP-2004", mb_eucjp2004_to_wchar, mb_wchar_to_eucjp2004, 1},
    {"EUC-CN", mb_euccn_to_wchar, mb_wchar_to_euccn, 0},
    {"EUC-TW", mb_euctw_to_wchar, mb_wchar_to_euctw, 2},
    {"EUC-KR", mb_euckr_to_wchar, mb_wchar_to_euckr, 0},
    {"UHC", mb_uhc_to_wchar, mb_wchar_to_uhc, 0},
    {"CP936", mb_cp936_to_wchar, mb_wchar_to_cp936, 0},
    {"BIG5", mb_big5_to_wchar, mb_wchar_to_big5, 0},
    {"CP950", mb_cp950_to_wchar, mb_wchar_to_cp950, 0},
};

static void encode(const codec *c, uint32_t *in, size_t n) {
    unsigned char out[64];
    mb_convert_buf b = {0};
    b.out = out; b.limit = out + sizeof(out);
    c->encode(in, n, &b, true);
    if (b.errors || b.out == out || b.out - out > 4) return;
    printf("E ");
    for (size_t i = 0; i < n; i++) printf("%08X", in[i]);
    putchar(' ');
    for (unsigned char *p = out; p < b.out; p++) printf("%02X", *p);
    putchar('\n');
}
static void decode(const codec *c, unsigned char *bytes, size_t len) {
    unsigned char *p = bytes;
    size_t left = len;
    unsigned int state = 0;
    uint32_t out[8];
    size_t n = c->decode(&p, &left, out, 8, &state);
    if (left || state || !n || n > 5) return;
    for (size_t i = 0; i < n; i++) if (out[i] > 0x10FFFF || (out[i] >= 0xD800 && out[i] <= 0xDFFF)) return;
    /* Reject two independent byte characters; a multi-scalar mapping consumes
     * one encoded character and is checked against proper prefixes below. */
    for (size_t k = 1; k < len; k++) {
        unsigned char *q = bytes; size_t rem = k; unsigned int st = 0; uint32_t prefix[8];
        size_t count = c->decode(&q, &rem, prefix, 8, &st);
        if (!rem && !st && count && prefix[0] <= 0x10FFFF) return;
    }
    printf("D ");
    for (size_t i = 0; i < len; i++) printf("%02X", bytes[i]);
    putchar(' ');
    for (size_t i = 0; i < n; i++) printf("%08X", out[i]);
    putchar('\n');
    if (n > 1) encode(c, out, n);
}
int main(int argc, char **argv) {
    if (argc != 2) return 2;
    const codec *c = NULL;
    for (size_t i = 0; i < sizeof(codecs)/sizeof(*codecs); i++) if (!strcmp(argv[1], codecs[i].name)) c = &codecs[i];
    if (!c) { fprintf(stderr, "unknown codec: %s\n", argv[1]); return 2; }
    unsigned char b[4];
    for (int a = 0; a < 256; a++) { b[0] = a; decode(c, b, 1); }
    if (c->euc != 3) for (int a = 0; a < 256; a++) for (int x = 0; x < 256; x++) { b[0] = a; b[1] = x; decode(c, b, 2); }
    if (c->euc == 1) for (int x = 0xA1; x <= 0xFE; x++) for (int y = 0xA1; y <= 0xFE; y++) { b[0] = 0x8F; b[1] = x; b[2] = y; decode(c, b, 3); }
    if (c->euc == 2) for (int p = 0xA1; p <= 0xB0; p++) for (int x = 0xA1; x <= 0xFE; x++) for (int y = 0xA1; y <= 0xFE; y++) { b[0] = 0x8E; b[1] = p; b[2] = x; b[3] = y; decode(c, b, 4); }
    for (uint32_t r = 0; r <= (c->euc == 3 ? 0xFFFF : 0x10FFFF); r++) if (r < 0xD800 || r > 0xDFFF) encode(c, &r, 1);
    return ferror(stdout) ? 1 : 0;
}
