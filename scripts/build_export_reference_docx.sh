#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
OUTPUT_PATH="${1:-$REPO_ROOT/config/export/reference.docx}"

mkdir -p "$(dirname -- "$OUTPUT_PATH")"

python3 - "$OUTPUT_PATH" <<'PY'
from __future__ import annotations

import io
import subprocess
import sys
import zipfile
import xml.etree.ElementTree as ET

OUTPUT_PATH = sys.argv[1]

W_NS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
R_NS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
A_NS = "http://schemas.openxmlformats.org/drawingml/2006/main"
PKG_REL_NS = "http://schemas.openxmlformats.org/package/2006/relationships"
CONTENT_TYPES_NS = "http://schemas.openxmlformats.org/package/2006/content-types"

ET.register_namespace("w", W_NS)
ET.register_namespace("r", R_NS)
ET.register_namespace("a", A_NS)

NS = {
    "w": W_NS,
    "r": R_NS,
    "a": A_NS,
    "pr": PKG_REL_NS,
    "ct": CONTENT_TYPES_NS,
}


def qn(namespace: str, tag: str) -> str:
    return f"{{{namespace}}}{tag}"


def parse_fragment(fragment: str):
    wrapper = ET.fromstring(
        f'<root xmlns:w="{W_NS}" xmlns:r="{R_NS}" xmlns:a="{A_NS}">{fragment}</root>'
    )
    return list(wrapper)


def replace_named_child(parent: ET.Element, local_name: str, new_child: ET.Element) -> None:
    child_tag = qn(W_NS, local_name)
    for child in list(parent):
        if child.tag == child_tag:
            parent.remove(child)
    parent.append(new_child)


def find_style(styles_root: ET.Element, style_id: str) -> ET.Element:
    for style in styles_root.findall("w:style", NS):
        if style.get(qn(W_NS, "styleId")) == style_id:
            return style
    raise KeyError(f"style {style_id} not found")


def ensure_style(styles_root: ET.Element, style_xml: str) -> ET.Element:
  style = parse_fragment(style_xml)[0]
  style_id = style.get(qn(W_NS, "styleId"))
  try:
    return find_style(styles_root, style_id)
  except KeyError:
    styles_root.append(style)
    return style


def update_style(styles_root: ET.Element, style_id: str, *, ppr: str | None = None, rpr: str | None = None) -> None:
    style = find_style(styles_root, style_id)
    if ppr is not None:
        replace_named_child(style, "pPr", parse_fragment(ppr)[0])
    if rpr is not None:
        replace_named_child(style, "rPr", parse_fragment(rpr)[0])


def ensure_font(font_root: ET.Element, font_name: str) -> None:
    for font in font_root.findall("w:font", NS):
        if font.get(qn(W_NS, "name")) == font_name:
            return
    ET.SubElement(font_root, qn(W_NS, "font"), {qn(W_NS, "name"): font_name})


def ensure_content_type_override(root: ET.Element, part_name: str, content_type: str) -> None:
    for override in root.findall("ct:Override", NS):
        if override.get("PartName") == part_name:
            override.set("ContentType", content_type)
            return
    ET.SubElement(
        root,
        qn(CONTENT_TYPES_NS, "Override"),
        {"PartName": part_name, "ContentType": content_type},
    )


def ensure_relationship(root: ET.Element, rel_id: str, rel_type: str, target: str) -> None:
    for rel in root.findall("pr:Relationship", NS):
        if rel.get("Id") == rel_id:
            rel.set("Type", rel_type)
            rel.set("Target", target)
            return
    ET.SubElement(
        root,
        qn(PKG_REL_NS, "Relationship"),
        {"Id": rel_id, "Type": rel_type, "Target": target},
    )


def serialize_xml(root: ET.Element, *, default_namespace: str | None = None) -> bytes:
    if default_namespace is not None:
        ET.register_namespace("", default_namespace)
    return ET.tostring(root, encoding="utf-8", xml_declaration=True)


reference_docx_bytes = subprocess.check_output(["pandoc", "--print-default-data-file", "reference.docx"])

with zipfile.ZipFile(io.BytesIO(reference_docx_bytes), "r") as source_zip:
    archive_entries = {name: source_zip.read(name) for name in source_zip.namelist()}

styles_root = ET.fromstring(archive_entries["word/styles.xml"])
doc_defaults = styles_root.find("w:docDefaults", NS)
if doc_defaults is None:
    raise RuntimeError("word/styles.xml is missing w:docDefaults")

replace_named_child(
    doc_defaults.find("w:rPrDefault", NS),
    "rPr",
    parse_fragment(
        """
        <w:rPr>
          <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" w:cs="Calibri" />
          <w:color w:val="111827" />
          <w:sz w:val="22" />
          <w:szCs w:val="22" />
          <w:lang w:val="en-US" w:eastAsia="zh-CN" w:bidi="ar-SA" />
        </w:rPr>
        """
    )[0],
)
replace_named_child(
    doc_defaults.find("w:pPrDefault", NS),
    "pPr",
    parse_fragment(
        """
        <w:pPr>
          <w:spacing w:before="0" w:after="140" w:line="360" w:lineRule="auto" />
        </w:pPr>
        """
    )[0],
)

update_style(
    styles_root,
    "Normal",
    ppr="""
      <w:pPr>
        <w:spacing w:before="0" w:after="140" w:line="360" w:lineRule="auto" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" w:cs="Calibri" />
        <w:color w:val="111827" />
        <w:sz w:val="22" />
        <w:szCs w:val="22" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "BodyText",
    ppr="""
      <w:pPr>
        <w:spacing w:before="0" w:after="140" w:line="360" w:lineRule="auto" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" w:cs="Calibri" />
        <w:color w:val="111827" />
        <w:sz w:val="22" />
        <w:szCs w:val="22" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "FirstParagraph",
    ppr="""
      <w:pPr>
        <w:spacing w:before="0" w:after="140" w:line="360" w:lineRule="auto" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" w:cs="Calibri" />
        <w:color w:val="111827" />
        <w:sz w:val="22" />
        <w:szCs w:val="22" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "Compact",
    ppr="""
      <w:pPr>
        <w:spacing w:before="0" w:after="0" w:line="300" w:lineRule="auto" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" w:cs="Calibri" />
        <w:color w:val="111827" />
        <w:sz w:val="21" />
        <w:szCs w:val="21" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "Title",
    ppr="""
      <w:pPr>
        <w:keepNext />
        <w:keepLines />
        <w:spacing w:before="0" w:after="240" w:line="320" w:lineRule="auto" />
        <w:jc w:val="center" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Microsoft YaHei" w:cs="Calibri" />
        <w:b />
        <w:color w:val="0F172A" />
        <w:sz w:val="64" />
        <w:szCs w:val="64" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "Subtitle",
    ppr="""
      <w:pPr>
        <w:keepNext />
        <w:keepLines />
        <w:spacing w:before="0" w:after="180" w:line="300" w:lineRule="auto" />
        <w:jc w:val="center" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Microsoft YaHei" w:cs="Calibri" />
        <w:color w:val="475569" />
        <w:sz w:val="28" />
        <w:szCs w:val="28" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "Author",
    ppr="""
      <w:pPr>
        <w:keepNext />
        <w:keepLines />
        <w:spacing w:before="0" w:after="60" />
        <w:jc w:val="center" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" w:cs="Calibri" />
        <w:color w:val="64748B" />
        <w:sz w:val="20" />
        <w:szCs w:val="20" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "Date",
    ppr="""
      <w:pPr>
        <w:keepNext />
        <w:keepLines />
        <w:spacing w:before="0" w:after="180" />
        <w:jc w:val="center" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" w:cs="Calibri" />
        <w:color w:val="64748B" />
        <w:sz w:val="20" />
        <w:szCs w:val="20" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "Heading1",
    ppr="""
      <w:pPr>
        <w:keepNext />
        <w:keepLines />
        <w:spacing w:before="400" w:after="140" />
        <w:pBdr>
          <w:bottom w:val="single" w:sz="8" w:space="1" w:color="DBEAFE" />
        </w:pBdr>
        <w:outlineLvl w:val="0" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Microsoft YaHei" w:cs="Calibri" />
        <w:b />
        <w:color w:val="1D4ED8" />
        <w:sz w:val="36" />
        <w:szCs w:val="36" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "Heading2",
    ppr="""
      <w:pPr>
        <w:keepNext />
        <w:keepLines />
        <w:spacing w:before="280" w:after="120" />
        <w:outlineLvl w:val="1" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Microsoft YaHei" w:cs="Calibri" />
        <w:b />
        <w:color w:val="2563EB" />
        <w:sz w:val="32" />
        <w:szCs w:val="32" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "Heading3",
    ppr="""
      <w:pPr>
        <w:keepNext />
        <w:keepLines />
        <w:spacing w:before="220" w:after="100" />
        <w:outlineLvl w:val="2" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Microsoft YaHei" w:cs="Calibri" />
        <w:b />
        <w:color w:val="0F172A" />
        <w:sz w:val="28" />
        <w:szCs w:val="28" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "Caption",
    ppr="""
      <w:pPr>
        <w:spacing w:before="60" w:after="100" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" w:cs="Calibri" />
        <w:i />
        <w:color w:val="64748B" />
        <w:sz w:val="20" />
        <w:szCs w:val="20" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "TableCaption",
    ppr="""
      <w:pPr>
        <w:keepNext />
        <w:spacing w:before="100" w:after="60" />
      </w:pPr>
    """,
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" w:cs="Calibri" />
        <w:i />
        <w:color w:val="64748B" />
        <w:sz w:val="20" />
        <w:szCs w:val="20" />
      </w:rPr>
    """,
)
update_style(
    styles_root,
    "VerbatimChar",
    rpr="""
      <w:rPr>
        <w:rFonts w:ascii="Consolas" w:hAnsi="Consolas" w:eastAsia="Microsoft YaHei" w:cs="Consolas" />
        <w:color w:val="111827" />
        <w:sz w:val="20" />
        <w:szCs w:val="20" />
      </w:rPr>
    """,
)
ensure_style(
    styles_root,
    """
      <w:style w:type="paragraph" w:customStyle="1" w:styleId="SourceCode">
        <w:name w:val="Source Code" />
        <w:basedOn w:val="Normal" />
        <w:link w:val="VerbatimChar" />
      </w:style>
    """,
)
update_style(
    styles_root,
    "SourceCode",
    ppr="""
      <w:pPr>
        <w:spacing w:before="100" w:after="100" w:line="300" w:lineRule="auto" />
        <w:shd w:val="clear" w:color="auto" w:fill="F8FAFC" />
        <w:pBdr>
          <w:top w:val="single" w:sz="8" w:space="1" w:color="D1D5DB" />
          <w:left w:val="single" w:sz="8" w:space="4" w:color="D1D5DB" />
          <w:bottom w:val="single" w:sz="8" w:space="1" w:color="D1D5DB" />
          <w:right w:val="single" w:sz="8" w:space="4" w:color="D1D5DB" />
        </w:pBdr>
        <w:wordWrap w:val="off" />
      </w:pPr>
    """,
)
update_style(
    styles_root,
    "Hyperlink",
    rpr="""
      <w:rPr>
        <w:color w:val="2563EB" />
        <w:u w:val="single" />
      </w:rPr>
    """,
)

table_style = find_style(styles_root, "Table")
for hidden_attr in ("semiHidden", "unhideWhenUsed"):
    attr_tag = qn(W_NS, hidden_attr)
    for child in list(table_style):
        if child.tag == attr_tag:
            table_style.remove(child)
table_style.insert(0, parse_fragment('<w:qFormat />')[0])
replace_named_child(
    table_style,
    "tblPr",
    parse_fragment(
        """
        <w:tblPr>
          <w:tblInd w:w="0" w:type="dxa" />
          <w:tblLayout w:type="autofit" />
          <w:tblCellMar>
            <w:top w:w="80" w:type="dxa" />
            <w:left w:w="108" w:type="dxa" />
            <w:bottom w:w="80" w:type="dxa" />
            <w:right w:w="108" w:type="dxa" />
          </w:tblCellMar>
          <w:tblBorders>
            <w:top w:val="single" w:sz="8" w:color="D1D5DB" />
            <w:left w:val="single" w:sz="8" w:color="D1D5DB" />
            <w:bottom w:val="single" w:sz="8" w:color="D1D5DB" />
            <w:right w:val="single" w:sz="8" w:color="D1D5DB" />
            <w:insideH w:val="single" w:sz="6" w:color="E5E7EB" />
            <w:insideV w:val="single" w:sz="6" w:color="E5E7EB" />
          </w:tblBorders>
        </w:tblPr>
        """
    )[0],
)

for tbl_style_type in ("firstRow", "band1Horz"):
    for child in list(table_style):
        if child.tag == qn(W_NS, "tblStylePr") and child.get(qn(W_NS, "type")) == tbl_style_type:
            table_style.remove(child)

table_style.extend(
    parse_fragment(
        """
        <w:tblStylePr w:type="firstRow">
          <w:tcPr>
            <w:shd w:val="clear" w:color="auto" w:fill="EAF2FF" />
            <w:tcBorders>
              <w:bottom w:val="single" w:sz="8" w:color="BFDBFE" />
            </w:tcBorders>
            <w:vAlign w:val="center" />
          </w:tcPr>
          <w:rPr>
            <w:b />
            <w:color w:val="0F172A" />
          </w:rPr>
        </w:tblStylePr>
        <w:tblStylePr w:type="band1Horz">
          <w:tcPr>
            <w:shd w:val="clear" w:color="auto" w:fill="F8FAFC" />
          </w:tcPr>
        </w:tblStylePr>
        """
    )
)

archive_entries["word/styles.xml"] = serialize_xml(styles_root)

theme_root = ET.fromstring(archive_entries["word/theme/theme1.xml"])
for scheme_name, latin_font, hans_font in (
    ("majorFont", "Calibri", "Microsoft YaHei"),
    ("minorFont", "Calibri", "SimSun"),
):
    scheme = theme_root.find(f".//a:{scheme_name}", NS)
    if scheme is None:
        continue
    latin = scheme.find("a:latin", NS)
    if latin is not None:
        latin.set("typeface", latin_font)
    hans = None
    for font in scheme.findall("a:font", NS):
        if font.get("script") == "Hans":
            hans = font
            break
    if hans is None:
        hans = ET.SubElement(scheme, qn(A_NS, "font"), {"script": "Hans"})
    hans.set("typeface", hans_font)
archive_entries["word/theme/theme1.xml"] = serialize_xml(theme_root)

font_table_root = ET.fromstring(archive_entries["word/fontTable.xml"])
for font_name in ("SimSun", "Microsoft YaHei", "Consolas"):
    ensure_font(font_table_root, font_name)
archive_entries["word/fontTable.xml"] = serialize_xml(font_table_root)

styles_root = ET.fromstring(archive_entries["word/styles.xml"])
latent_styles = styles_root.find("w:latentStyles", NS)
if latent_styles is not None:
    for child in list(latent_styles):
        attrs = child.attrib
        if attrs.get(qn(W_NS, "defLockedState")) == "0":
            continue
        for attr_name in ("semiHidden", "unhideWhenUsed"):
            attr_q = qn(W_NS, attr_name)
            if attr_q in attrs:
                del attrs[attr_q]
        attrs[qn(W_NS, "defQFormat")] = "0"
    archive_entries["word/styles.xml"] = serialize_xml(styles_root)

settings_root = ET.fromstring(archive_entries["word/settings.xml"])
theme_font_lang = settings_root.find("w:themeFontLang", NS)
if theme_font_lang is None:
    theme_font_lang = ET.SubElement(settings_root, qn(W_NS, "themeFontLang"))
theme_font_lang.set(qn(W_NS, "val"), "en-US")
theme_font_lang.set(qn(W_NS, "eastAsia"), "zh-CN")

style_pane_filter = settings_root.find("w:stylePaneFormatFilter", NS)
if style_pane_filter is None:
    style_pane_filter = ET.SubElement(settings_root, qn(W_NS, "stylePaneFormatFilter"))
style_pane_filter.set(qn(W_NS, "val"), "0000")

update_fields = settings_root.find("w:updateFields", NS)
if update_fields is None:
    update_fields = ET.SubElement(settings_root, qn(W_NS, "updateFields"))
update_fields.set(qn(W_NS, "val"), "false")
archive_entries["word/settings.xml"] = serialize_xml(settings_root)

document_root = ET.fromstring(archive_entries["word/document.xml"])
body = document_root.find("w:body", NS)
if body is None:
    raise RuntimeError("word/document.xml is missing w:body")

existing_footnote = None
for child in list(body):
    if child.tag == qn(W_NS, "sectPr"):
        body.remove(child)
        existing_footnote = child.find("w:footnotePr", NS)

sect_pr = parse_fragment(
    """
    <w:sectPr>
      <w:footerReference w:type="default" r:id="rIdFooter1" />
      <w:pgSz w:w="11906" w:h="16838" />
      <w:pgMar w:top="1440" w:right="1200" w:bottom="1440" w:left="1200" w:header="720" w:footer="720" w:gutter="0" />
      <w:pgNumType w:start="1" />
      <w:cols w:space="720" />
      <w:docGrid w:linePitch="312" />
    </w:sectPr>
    """
)[0]
if existing_footnote is not None:
    sect_pr.append(existing_footnote)
body.append(sect_pr)
archive_entries["word/document.xml"] = serialize_xml(document_root)

rels_root = ET.fromstring(archive_entries["word/_rels/document.xml.rels"])
ensure_relationship(
    rels_root,
    "rIdFooter1",
    "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer",
    "footer1.xml",
)
archive_entries["word/_rels/document.xml.rels"] = serialize_xml(rels_root, default_namespace=PKG_REL_NS)

content_types_root = ET.fromstring(archive_entries["[Content_Types].xml"])
ensure_content_type_override(
    content_types_root,
    "/word/footer1.xml",
    "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml",
)
archive_entries["[Content_Types].xml"] = serialize_xml(content_types_root, default_namespace=CONTENT_TYPES_NS)

archive_entries["word/footer1.xml"] = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:p>
    <w:pPr>
      <w:jc w:val="right" />
      <w:spacing w:before="0" w:after="0" />
      <w:pBdr>
        <w:top w:val="single" w:sz="6" w:space="1" w:color="D1D5DB" />
      </w:pBdr>
    </w:pPr>
    <w:r>
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" />
        <w:color w:val="64748B" />
        <w:sz w:val="18" />
        <w:szCs w:val="18" />
      </w:rPr>
      <w:t>第 </w:t>
    </w:r>
    <w:fldSimple w:instr=" PAGE ">
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" />
          <w:color w:val="64748B" />
          <w:sz w:val="18" />
          <w:szCs w:val="18" />
        </w:rPr>
        <w:t>1</w:t>
      </w:r>
    </w:fldSimple>
    <w:r>
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" />
        <w:color w:val="64748B" />
        <w:sz w:val="18" />
        <w:szCs w:val="18" />
      </w:rPr>
      <w:t> 页 / 共 </w:t>
    </w:r>
    <w:fldSimple w:instr=" NUMPAGES ">
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" />
          <w:color w:val="64748B" />
          <w:sz w:val="18" />
          <w:szCs w:val="18" />
        </w:rPr>
        <w:t>1</w:t>
      </w:r>
    </w:fldSimple>
    <w:r>
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="SimSun" />
        <w:color w:val="64748B" />
        <w:sz w:val="18" />
        <w:szCs w:val="18" />
      </w:rPr>
      <w:t> 页</w:t>
    </w:r>
  </w:p>
</w:ftr>
'''.encode("utf-8")

with zipfile.ZipFile(OUTPUT_PATH, "w", compression=zipfile.ZIP_DEFLATED) as target_zip:
    for name, data in archive_entries.items():
        target_zip.writestr(name, data)
PY