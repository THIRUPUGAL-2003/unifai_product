import os
import sys
from pathlib import Path
from reportlab.lib.pagesizes import A4
from reportlab.lib import colors
from reportlab.lib.styles import ParagraphStyle
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Image, Table, TableStyle, PageBreak
)
from reportlab.pdfgen import canvas

BASE_DIR = Path(r"d:\unifai_project")
ASSETS_DIR = BASE_DIR / "docs" / "pdf_assets"
EXTRACTED_DIR = BASE_DIR / "docs" / "extracted_from_new_doc"
LOGO_PATH = str(BASE_DIR / "ui" / "public" / "yes-panchi-logo.png")
OUTPUT_PDF = Path(r"C:\Users\sakth\Downloads\infosec.pdf")
LOCAL_COPY = BASE_DIR / "docs" / "YP_RAKSHA_InfoSec_Compliance_Report.pdf"

# Dimensions
PAGE_W, PAGE_H = A4  # 595.27 x 841.89 pt
CONTENT_W = 523.27   # 595.27 - 72

# Colors
NAVY = colors.HexColor("#0F172A")        # Slate 900
SLATE_DARK = colors.HexColor("#1E293B")  # Slate 800
SLATE_BODY = colors.HexColor("#334155")  # Slate 700
MUTED = colors.HexColor("#64748B")       # Slate 500
GREEN = colors.HexColor("#059669")       # Emerald 600
GREEN_BG = colors.HexColor("#F0FDF4")    # Emerald 50
GREEN_BORDER = colors.HexColor("#86EFAC")# Emerald 300
CARD_BG = colors.HexColor("#F8FAFC")     # Slate 50
BORDER_COLOR = colors.HexColor("#E2E8F0")# Slate 200
IMG_BORDER = colors.HexColor("#CBD5E1")  # Slate 300
WHITE = colors.white

def get_styles():
    return {
        # Cover Styles
        "cover_company": ParagraphStyle("cover_company", fontName="Helvetica-Bold", fontSize=18, leading=22, textColor=NAVY, alignment=1, spaceAfter=8),
        "cover_product": ParagraphStyle("cover_product", fontName="Helvetica-Bold", fontSize=44, leading=50, textColor=NAVY, alignment=1, spaceAfter=3),
        "cover_version": ParagraphStyle("cover_version", fontName="Helvetica-Bold", fontSize=11, leading=15, textColor=MUTED, alignment=1, spaceAfter=8),
        "cover_expansion": ParagraphStyle("cover_expansion", fontName="Helvetica-Bold", fontSize=12, leading=16, textColor=GREEN, alignment=1, spaceAfter=8),
        "cover_tagline": ParagraphStyle("cover_tagline", fontName="Helvetica", fontSize=11, leading=15, textColor=MUTED, alignment=1, spaceAfter=15),
        
        # General Titles
        "h1": ParagraphStyle("h1", fontName="Helvetica-Bold", fontSize=12, leading=15, textColor=NAVY, spaceAfter=2),
        "h2_topic": ParagraphStyle("h2_topic", fontName="Helvetica-Bold", fontSize=8.5, leading=11, textColor=NAVY, spaceAfter=1),
        "index_title": ParagraphStyle("index_title", fontName="Helvetica-Bold", fontSize=18, leading=22, textColor=NAVY, alignment=1, spaceAfter=6),
        "body": ParagraphStyle("body", fontName="Helvetica", fontSize=6.8, leading=8.8, textColor=SLATE_BODY, spaceAfter=2),
        
        # Requirement Styles
        "req_badge": ParagraphStyle("req_badge", fontName="Helvetica-Bold", fontSize=6.2, leading=7.8, textColor=GREEN),
        "req_text": ParagraphStyle("req_text", fontName="Helvetica-Oblique", fontSize=6.8, leading=8.6, textColor=SLATE_DARK),
        
        # Captions
        "caption": ParagraphStyle("caption", fontName="Helvetica-Bold", fontSize=6.2, leading=8.0, textColor=MUTED, alignment=1),
        
        # Table of Contents Styles
        "toc_th": ParagraphStyle("toc_th", fontName="Helvetica-Bold", fontSize=7.5, leading=10, textColor=WHITE, alignment=0),
        "toc_th_center": ParagraphStyle("toc_th_center", fontName="Helvetica-Bold", fontSize=7.5, leading=10, textColor=WHITE, alignment=1),
        "toc_td_sec": ParagraphStyle("toc_td_sec", fontName="Helvetica-Bold", fontSize=6.5, leading=8.5, textColor=NAVY, alignment=1),
        "toc_td_title": ParagraphStyle("toc_td_title", fontName="Helvetica", fontSize=6.5, leading=8.5, textColor=SLATE_DARK),
        "toc_td_page": ParagraphStyle("toc_td_page", fontName="Helvetica-Bold", fontSize=6.5, leading=8.5, textColor=NAVY, alignment=1),
    }

ST = get_styles()

class NumberedCanvas(canvas.Canvas):
    """Two-pass canvas to compute dynamic total pages and draw clean running headers & footers."""
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._saved_page_states = []

    def showPage(self):
        self._saved_page_states.append(dict(self.__dict__))
        self._startPage()

    def save(self):
        num_pages = len(self._saved_page_states)
        for state in self._saved_page_states:
            self.__dict__.update(state)
            self.draw_decorations(num_pages)
            super().showPage()
        super().save()

    def draw_decorations(self, total_pages):
        page_num = self._pageNumber
        self.saveState()

        # Running Header (Pages 2 through total_pages)
        if page_num > 1:
            try:
                self.drawImage(LOGO_PATH, 36, PAGE_H - 36, width=65, height=20, preserveAspectRatio=True, mask='auto')
            except Exception:
                self.setFont("Helvetica-Bold", 8)
                self.setFillColor(NAVY)
                self.drawString(36, PAGE_H - 28, "YES PANCHI")

            self.setFont("Helvetica-Bold", 8)
            self.setFillColor(SLATE_DARK)
            self.drawRightString(PAGE_W - 36, PAGE_H - 24, "YP-Raksha  |  Security & Compliance Verification")
            
            self.setFont("Helvetica", 7)
            self.setFillColor(MUTED)
            self.drawRightString(PAGE_W - 36, PAGE_H - 33, "Enterprise AI Security & Data Governance Gateway")

            self.setStrokeColor(BORDER_COLOR)
            self.setLineWidth(0.6)
            self.line(36, PAGE_H - 39, PAGE_W - 36, PAGE_H - 39)

            # Running Footer (Pages 2 through total_pages)
            self.setStrokeColor(BORDER_COLOR)
            self.setLineWidth(0.6)
            self.line(36, 36, PAGE_W - 36, 36)

            # Footer Left: Brand & Product
            self.setFont("Helvetica-Bold", 8)
            self.setFillColor(NAVY)
            self.drawString(36, 23, "YES PANCHI")
            
            self.setFont("Helvetica", 8)
            self.setFillColor(MUTED)
            self.drawString(92, 23, " |  YP-Raksha")

            # Footer Right: Dynamic Page Number
            self.setFont("Helvetica-Bold", 8)
            self.setFillColor(NAVY)
            self.drawRightString(PAGE_W - 36, 23, f"Page {page_num} of {total_pages}")

        self.restoreState()

def make_req_box(label, req_text):
    badge = Paragraph(f"<b>{label}</b> &nbsp;|&nbsp; <b>MANDATORY SECURITY SPECIFICATION</b>", ST["req_badge"])
    req = Paragraph(f'"{req_text}"', ST["req_text"])
    t = Table([[badge], [req]], colWidths=[CONTENT_W])
    t.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, -1), GREEN_BG),
        ("BOX", (0, 0), (-1, -1), 0.5, GREEN_BORDER),
        ("LINELEFT", (0, 0), (0, -1), 2.5, GREEN),
        ("TOPPADDING", (0, 0), (-1, -1), 1),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 1),
        ("LEFTPADDING", (0, 0), (-1, -1), 5),
        ("RIGHTPADDING", (0, 0), (-1, -1), 5),
    ]))
    return t

def make_image_card(img_path, width=430, height=204, caption_text=""):
    img = Image(str(img_path), width=width, height=height)
    cap = Paragraph(caption_text, ST["caption"])
    t = Table([[img], [cap]], colWidths=[width + 8])
    t.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, -1), CARD_BG),
        ("BOX", (0, 0), (-1, -1), 0.5, IMG_BORDER),
        ("ALIGN", (0, 0), (-1, -1), "CENTER"),
        ("VALIGN", (0, 0), (-1, -1), "MIDDLE"),
        ("TOPPADDING", (0, 0), (-1, -1), 2),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 2),
        ("LEFTPADDING", (0, 0), (-1, -1), 4),
        ("RIGHTPADDING", (0, 0), (-1, -1), 4),
    ]))
    return t

def add_paired_page(story, page_title, item1, item2, img_w=430, img_h=204):
    story.append(PageBreak())
    story.append(Paragraph(page_title, ST["h1"]))
    story.append(Spacer(1, 1))
    
    # Topic 1
    t1_title, t1_label, t1_req, t1_desc, t1_img, t1_cap = item1
    story.append(Paragraph(f"<b>{t1_title}</b>", ST["h2_topic"]))
    story.append(make_req_box(t1_label, t1_req))
    story.append(Paragraph(f"<b>Technical Compliance & Architecture:</b> {t1_desc}", ST["body"]))
    story.append(make_image_card(t1_img, img_w, img_h, t1_cap))
    story.append(Spacer(1, 3))
    
    # Topic 2
    t2_title, t2_label, t2_req, t2_desc, t2_img, t2_cap = item2
    story.append(Paragraph(f"<b>{t2_title}</b>", ST["h2_topic"]))
    story.append(make_req_box(t2_label, t2_req))
    story.append(Paragraph(f"<b>Technical Compliance & Architecture:</b> {t2_desc}", ST["body"]))
    story.append(make_image_card(t2_img, img_w, img_h, t2_cap))

def build_pdf():
    story = []

    # ==========================================
    # PAGE 1: COVER PAGE
    # ==========================================
    story.append(Spacer(1, 160))
    try:
        logo_img = Image(LOGO_PATH, width=220, height=70)
        logo_img.hAlign = 'CENTER'
        story.append(logo_img)
    except Exception as e:
        story.append(Paragraph("<b>YES PANCHI</b>", ST["cover_company"]))
    
    story.append(Spacer(1, 30))
    story.append(Paragraph("YES PANCHI", ST["cover_company"]))
    story.append(Spacer(1, 4))
    story.append(Paragraph("YP-Raksha", ST["cover_product"]))
    story.append(Spacer(1, 2))
    story.append(Paragraph("Version 1.5", ST["cover_version"]))
    story.append(Spacer(1, 6))
    story.append(Paragraph("RAKSHA - Real-time AI Knowledge Screening & Hazard Audit", ST["cover_expansion"]))
    story.append(Spacer(1, 6))
    story.append(Paragraph("Enterprise AI Security & Data Governance", ST["cover_tagline"]))

    # ==========================================
    # PAGE 2: TABLE OF CONTENTS (INDEX)
    # ==========================================
    story.append(PageBreak())
    story.append(Spacer(1, 5))
    story.append(Paragraph("Index", ST["index_title"]))
    story.append(Spacer(1, 2))

    toc_headers = ["Section / Req", "Topic / Security Control Specification", "Page Number"]
    toc_rows = [
        ["6.3.8 VI (a)", "Automatic Redaction — Inline PII masking & redaction (Aadhaar, PAN, contact details)", "Page 3"],
        ["6.3.8 VI (b)", "Malicious Intent Detection — Mass extraction, enumeration patterns & jailbreak defense", "Page 4"],
        ["6.3.8 VI (c)", "Prompt Sanitization — Incoming query analysis & classified personal data protection", "Page 4"],
        ["6.3.8 VI (d)", "Clipboard & Data Governance — Strict controls on Copy-Paste, Drag-and-Drop & Downloads", "Page 5"],
        ["6.3.8 VI (e)", "Policy Enforcement — Purpose alignment, consent validation & minor/children data", "Page 5"],
        ["6.3.8 VI (f)", "Just-in-Time (JIT) Coaching — Real-time browser-level alerts & employee education popups", "Page 6"],
        ["6.3.8 VI (g)", "Response Scanning — AI-generated response analysis & classification metadata", "Page 6"],
        ["6.3.8 VI (h)", "Risk Scoring — Regulatory & business risk scoring for AI responses & search", "Page 7"],
        ["Req 44 & 45", "AI Discovery & Tool Visibility — Automated tool discovery across 52+ domains & enterprise inventory", "Page 8"],
        ["Req 46 & 47", "Shadow AI & Data Repositories — Shadow AI domain locking & file repository transmission controls", "Page 9"],
        ["Req 48 & 49", "Continuous Monitoring & DLP — Continuous prompt interaction logging & regulated data detection", "Page 10"],
        ["Req 50 & 51", "Policy Controls & Enforcement — Policy-based data exposure restrictions & automated enforcement", "Page 11"],
        ["Req 52 & 53", "Data Identification & Classification — Sensitive enterprise data detection & automated data classification", "Page 12"],
        ["Req 54 & 55", "Risk Scoring & Access Boundaries — Data-sensitivity AI risk scoring & excessive query detection", "Page 13"],
        ["Req 56 & 57", "Dataset Access & Anomaly Detection — Restricted dataset prevention policies & anomalous pattern monitoring", "Page 14"],
        ["Req 58 & 59", "Suspicious Data & Exfiltration Alerts — Suspicious AI interaction detection & automated exfiltration alerts", "Page 15"],
        ["Req 60 & 61", "Scraping Mitigation & Workflows — Automated prompt scraping mitigation & analyst investigation tools", "Page 16"],
        ["Req 62 & 63", "Audit Logging & Governance Controls — Detailed immutable AI audit logs & policy-aligned workspace governance", "Page 17"],
        ["Req 64 & 65", "Compliance Reports & Risk Posture — AI compliance telemetry reports & fleet risk posture dashboards", "Page 18"],
        ["Req 66 & 67", "Audit Reporting & Dataset Protection — Search activity audit reporting & classified dataset access prevention", "Page 19"],
        ["Req 68 & 69", "Runtime Monitoring & Threat Defense — Runtime proxy data inspection & prompt injection attack defense", "Page 20"],
        ["Req 70 & 71", "Output Protection & Remediation — Sensitive token output leakage prevention & automated blocking actions", "Page 21"],
        ["Req 72 & 73", "Multi-Asset Risk & Adoption Trends — Workstation hardware telemetry & enterprise AI adoption analytics", "Page 22"],
        ["Req 74 & 75", "Behavioral Scoring & Executive Reports — Behavioral contextual risk scoring & executive leadership briefings", "Page 23"],
    ]

    t_data = [
        [Paragraph(toc_headers[0], ST["toc_th_center"]),
         Paragraph(toc_headers[1], ST["toc_th"]),
         Paragraph(toc_headers[2], ST["toc_th_center"])]
    ]
    for row in toc_rows:
        t_data.append([
            Paragraph(f"<b>{row[0]}</b>", ST["toc_td_sec"]),
            Paragraph(f"<b>{row[1].split(' — ')[0]}</b>" + (f"<br/><font color='#64748B'>{row[1].split(' — ')[1]}</font>" if ' — ' in row[1] else ""), ST["toc_td_title"]),
            Paragraph(f"<b>{row[2]}</b>", ST["toc_td_page"]),
        ])
    
    toc_table = Table(t_data, colWidths=[85, 345, 93], repeatRows=1)
    toc_table.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), NAVY),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [WHITE, CARD_BG]),
        ("GRID", (0, 0), (-1, -1), 0.5, BORDER_COLOR),
        ("VALIGN", (0, 0), (-1, -1), "MIDDLE"),
        ("TOPPADDING", (0, 0), (-1, -1), 2),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 2),
        ("LEFTPADDING", (0, 0), (-1, -1), 5),
        ("RIGHTPADDING", (0, 0), (-1, -1), 5),
    ]))
    story.append(toc_table)

    # ==========================================
    # PAGE 3: TOPIC 6.3.8 VI (a)
    # ==========================================
    story.append(PageBreak())
    story.append(Paragraph("6.3.8 VI (a): Automatic Redaction", ST["h1"]))
    story.append(make_req_box("6.3.8 VI (a)", "Automatic Redaction: The solution must automatically block or redact PII (e.g., Aadhaar, PAN, contact details) from natural language prompts before transmission to any AI service."))
    story.append(Spacer(1, 2))
    desc_a = (
        "YP-Raksha intercepts prompt submissions at the browser runtime and proxy layer before payload dispatch. "
        "High-performance regex evaluators and named-entity recognition (NER) models scan the buffer for Indian and international PII formats, "
        "ensuring zero leakage to external LLMs."
    )
    story.append(Paragraph(f"<b>Technical Compliance & Architecture:</b> {desc_a}", ST["body"]))
    story.append(Spacer(1, 2))
    story.append(make_image_card(ASSETS_DIR / "p1_img1.jpeg", 430, 204, "Figure 1.1: YP-Raksha Guard Rules Engine with active PII redaction patterns for Aadhaar, PAN, and Contact Details."))
    story.append(Spacer(1, 3))
    story.append(make_image_card(ASSETS_DIR / "p1_img2.jpeg", 430, 204, "Figure 1.2: Audit verification log displaying automatic inline masking applied to employee prompt before proxy transmission."))

    # ==========================================
    # PAGE 4: TOPICS 6.3.8 VI (b) & (c)
    # ==========================================
    story.append(PageBreak())
    story.append(Paragraph("6.3.8 VI (b) & (c): Malicious Intent Detection & Prompt Sanitization", ST["h1"]))
    story.append(Paragraph("<b>Malicious Intent Detection</b>", ST["h2_topic"]))
    story.append(make_req_box("6.3.8 VI (b)", "The solution must detect and block queries exhibiting malicious intent, such as mass extraction attempts, enumeration, and adversarial jailbreaks."))
    story.append(Paragraph("<b>Technical Compliance:</b> YP-Raksha deploys heuristic token evaluators to intercept jailbreak attempts (DAN, prompt override) and token velocity rate limiters.", ST["body"]))
    story.append(make_image_card(ASSETS_DIR / "p2_img1.jpeg", 430, 204, "Figure 2.1: Interception of adversarial jailbreak and mass enumeration attempts at the gateway layer."))
    story.append(Spacer(1, 3))
    story.append(Paragraph("<b>Prompt Sanitization</b>", ST["h2_topic"]))
    story.append(make_req_box("6.3.8 VI (c)", "The solution must inspect and sanitize incoming prompts to prevent the injection of classified, confidential, or sensitive personal data into public AI services."))
    story.append(Paragraph("<b>Technical Compliance:</b> Scans input streams for proprietary source code, internal IPs, connection strings, and classified operational assets before dispatch.", ST["body"]))
    story.append(make_image_card(ASSETS_DIR / "p2_img2.jpeg", 430, 204, "Figure 2.2: Prompt sanitization inspection log showing scrubbed credentials and sanitized technical payloads."))

    # ==========================================
    # PAGE 5: TOPICS 6.3.8 VI (d) & (e)
    # ==========================================
    story.append(PageBreak())
    story.append(Paragraph("6.3.8 VI (d) & (e): Clipboard Governance & Policy Enforcement", ST["h1"]))
    story.append(Paragraph("<b>Clipboard & Data Governance</b>", ST["h2_topic"]))
    story.append(make_req_box("6.3.8 VI (d)", "The solution must enforce strict controls to prevent unauthorized copy-paste, drag-and-drop, or bulk insertion of sensitive enterprise data into AI interfaces."))
    story.append(Paragraph("<b>Technical Compliance:</b> In-browser DOM event listeners hook clipboard operations, terminating unauthorized copy-paste or file drop.", ST["body"]))
    story.append(make_image_card(ASSETS_DIR / "p3_img1.jpeg", 430, 204, "Figure 3.1: Browser Settings console enforcing strict Copy-Paste, Drag-and-Drop, and File Download blocking."))
    story.append(Spacer(1, 3))
    story.append(Paragraph("<b>Policy Enforcement</b>", ST["h2_topic"]))
    story.append(make_req_box("6.3.8 VI (e)", "The solution must validate that AI requests align with declared enterprise purpose and block prompts involving expired/withdrawn consent or children's data."))
    story.append(Paragraph("<b>Technical Compliance:</b> Validates session tokens and prompt context against purpose tags; prompts referencing minors are dropped.", ST["body"]))
    story.append(make_image_card(ASSETS_DIR / "p3_img2.jpeg", 430, 204, "Figure 3.2: Real-time policy enforcement engine terminating prompt submission due to missing consent authorization."))

    # ==========================================
    # PAGE 6: TOPICS 6.3.8 VI (f) & (g)
    # ==========================================
    story.append(PageBreak())
    story.append(Paragraph("6.3.8 VI (f) & (g): Just-in-Time (JIT) Coaching & Response Scanning", ST["h1"]))
    story.append(Paragraph("<b>Just-in-Time (JIT) Coaching</b>", ST["h2_topic"]))
    story.append(make_req_box("6.3.8 VI (f)", "The solution must deliver real-time, context-aware policy alerts directly to users upon violation to reinforce safe AI usage."))
    story.append(Paragraph("<b>Technical Compliance:</b> Dynamic floating alert banners inform users why their interaction violated policy, providing educational coaching.", ST["body"]))
    story.append(make_image_card(ASSETS_DIR / "p4_img1.jpeg", 430, 204, "Figure 4.1: Real-time JIT coaching banner displayed to employee explaining policy violation at the moment of entry."))
    story.append(Spacer(1, 3))
    story.append(Paragraph("<b>Response Scanning</b>", ST["h2_topic"]))
    story.append(make_req_box("6.3.8 VI (g)", "All AI-generated responses must be analysed for PII or sensitive content and evaluated against established classification metadata."))
    story.append(Paragraph("<b>Technical Compliance:</b> Captures inbound LLM completions, inspecting for toxicity, hallucinated instructions, and attaching safety metadata.", ST["body"]))
    story.append(make_image_card(ASSETS_DIR / "p4_img2.jpeg", 430, 204, "Figure 4.2: AI Response Scanning dashboard showing real-time toxicity analysis and compliance metadata tagging."))

    # ==========================================
    # PAGE 7: TOPIC 6.3.8 VI (h)
    # ==========================================
    story.append(PageBreak())
    story.append(Paragraph("6.3.8 VI (h): Risk Scoring", ST["h1"]))
    story.append(make_req_box("6.3.8 VI (h)", "Risk Scoring: The solution shall assign a regulatory and business risk score to each AI-generated response, search result, and synthesized output, enabling risk-based blocking or human review."))
    story.append(Spacer(1, 2))
    desc_h = (
        "YP-Raksha calculates a multidimensional composite risk score (0 to 100) weighing data sensitivity, user role, platform sensitivity, "
        "and prompt toxicity. Interactions exceeding tolerance thresholds trigger automated containment (Low: Allowed, Medium: JIT Alert, High: Hard Block)."
    )
    story.append(Paragraph(f"<b>Technical Compliance & Architecture:</b> {desc_h}", ST["body"]))
    story.append(Spacer(1, 2))
    story.append(make_image_card(ASSETS_DIR / "p5_img1.jpeg", 430, 204, "Figure 5.1: Multi-factor Risk Scoring engine matrix mapping weights across data sensitivity and platform security."))
    story.append(Spacer(1, 3))
    story.append(make_image_card(ASSETS_DIR / "p5_img2.jpeg", 430, 204, "Figure 5.2: Detailed Interaction Log and Risk Scoring table assigning risk ratings to every prompt and response."))

    # ==========================================
    # PAGES 8 - 23: REQUIREMENTS 44 TO 75 (2 PER PAGE, FULL SIZE SCREENSHOTS)
    # ==========================================
    pairs = [
        # Page 8: Requirements 44 & 45
        ("AI Discovery & Tool Visibility",
         ("Automated AI Tools Discovery", "AI DISCOVERY SPECIFICATION",
          "The solution must automatically discover AI tools, AI agents, AI chatbots, and AI model integrations used across the enterprise environment.",
          "Continuous discovery pipeline identifies enterprise access across 52+ commercial and emerging AI domains including ChatGPT, Claude, and Gemini.",
          EXTRACTED_DIR / "18_image22.png",
          "Figure 6.1: Target Web AI Platforms console showing automated discovery and active monitoring across 52 AI domains."),
         ("Enterprise-Wide AI Tool Visibility", "VISIBILITY SPECIFICATION",
          "The solution should provide visibility into AI tool usage across users, applications, and data repositories.",
          "Centralized endpoint inventory tracks desktop agents and browser extensions, logging machine hostname, IP, MAC address, and active user accounts.",
          EXTRACTED_DIR / "11_image23.png",
          "Figure 6.2: Centralized Guard Agents management dashboard tracking endpoint hostnames, IP, MAC, version, and status.")),

        # Page 9: Requirements 46 & 47
        ("Shadow AI & Data Repositories",
         ("Shadow AI Identification & Containment", "SHADOW AI SPECIFICATION",
          "The solution must identify shadow AI usage, including unauthorized AI applications and browser-based AI tools.",
          "Detects unapproved AI domains and browser AI tools; enables administrators to immediately enforce domain-level locking (Block entire website).",
          EXTRACTED_DIR / "27_image24.png",
          "Figure 7.1: Monitored AI Domains dashboard featuring one-click 'Block entire website' toggle to contain shadow AI."),
         ("Data Repositories Interaction Monitoring", "REPOSITORY MONITORING SPECIFICATION",
          "The solution should monitor AI interactions with enterprise data repositories including file systems, collaboration platforms, and cloud storage.",
          "Monitors file uploads, cloud attachments, and document drag-and-drop into external AI services, blocking unauthorized transmissions.",
          EXTRACTED_DIR / "04_image25.png",
          "Figure 7.2: Connected enterprise data repositories and cloud connector proxy monitoring console.")),

        # Page 10: Requirements 48 & 49
        ("Continuous Monitoring & DLP",
         ("Continuous User Interaction Monitoring", "RUNTIME MONITORING SPECIFICATION",
          "The solution must provide continuous monitoring of user interactions with AI tools to detect potential data leakage risks.",
          "Every prompt submission is intercepted inline, recording user prompts, tokens, execution cost, and security actions in live audit logs.",
          EXTRACTED_DIR / "23_image26.png",
          "Figure 8.1: Prompt & Chat History audit log displaying live intercepted requests, tokens, costs, and policy actions."),
         ("Sensitive & Regulated Data Detection", "DATA DETECTION SPECIFICATION",
          "The solution should detect when sensitive or regulated data is being accessed or transmitted through AI applications.",
          "High-speed pattern engine flags Aadhaar, mobile numbers, PAN, 18+ content, and confidential company tokens across all prompt dispatches.",
          EXTRACTED_DIR / "16_image27.png",
          "Figure 8.2: Active DLP Guard Rules console enforcing automated classification, 18+ content filters, and PII masking.")),

        # Page 11: Requirements 50 & 51
        ("Policy Controls & Enforcement",
         ("Policy-Based Data Exposure Controls", "POLICY CONTROL SPECIFICATION",
          "The solution must provide policy-based controls to restrict the exposure of sensitive data to AI tools and services.",
          "Enforces granular policies (Block vs. Redact) preventing classified datasets and confidential employee credentials from reaching external LLMs.",
          ASSETS_DIR / "p3_img1.jpeg",
          "Figure 9.1: Browser Guard Settings console enforcing strict copy-paste, drag-and-drop, and download exposure controls."),
         ("Automated Policy Enforcement Engine", "AUTOMATED ENFORCEMENT SPECIFICATION",
          "The solution should support automated enforcement of AI data protection policies.",
          "Enforces policy decisions within 5 milliseconds at runtime, terminating unauthorized operations without requiring manual analyst intervention.",
          EXTRACTED_DIR / "16_image27.png",
          "Figure 9.2: Guard Rules policy manager mapping critical severity actions (Block vs. Redact) across enterprise targets.")),

        # Page 12: Requirements 52 & 53
        ("Data Identification & Classification",
         ("Sensitive Enterprise Data Identification", "DATA IDENTIFICATION SPECIFICATION",
          "The solution must identify sensitive enterprise data accessed by AI systems, including personal data, financial data, and intellectual property.",
          "Detects proprietary intellectual property, customer financial records, and PII before text is uploaded into third-party AI interfaces.",
          ASSETS_DIR / "p1_img2.jpeg",
          "Figure 10.1: Live interaction audit trail identifying personal identifiers and proprietary corporate tokens."),
         ("Automated Data Classification Mechanisms", "CLASSIFICATION SPECIFICATION",
          "The solution should classify sensitive data accessed by AI systems using automated classification mechanisms.",
          "Assigns contextual data classifications (Confidential, Restricted, PII) in real time and masks detected tokens prior to proxy transmission.",
          EXTRACTED_DIR / "12_image28.png",
          "Figure 10.2: Guard Insights telemetry dashboard showing automated classification trends across enterprise prompts.")),

        # Page 13: Requirements 54 & 55
        ("Risk Scoring & Access Boundaries",
         ("Data-Sensitivity-Driven AI Risk Scoring", "RISK SCORING SPECIFICATION",
          "The solution must provide risk scoring for AI systems based on the sensitivity of data they access.",
          "Computes continuous risk scores reflecting the sensitivity of data repositories accessed by each AI system or employee profile.",
          EXTRACTED_DIR / "28_image29.png",
          "Figure 11.1: LLM Interaction Log tracking request tokens, latency, cost metrics, and dynamic risk scoring."),
         ("Excessive & Unnecessary Data Access Detection", "ANOMALY DETECTION SPECIFICATION",
          "The solution should detect when AI tools attempt to access excessive or unnecessary data.",
          "Monitors query volume and scope; excessive requests trigger instant JIT notification banners and prompt rate-limiting.",
          ASSETS_DIR / "p2_img1.jpeg",
          "Figure 11.2: Gateway interception of excessive query bursts, jailbreak patterns, and mass data extraction.")),

        # Page 14: Requirements 56 & 57
        ("Dataset Access & Anomaly Detection",
         ("Policy-Based AI Dataset Access Restriction", "DATASET RESTRICTION SPECIFICATION",
          "The solution must provide the ability to restrict AI model access to sensitive datasets based on predefined policies.",
          "Restricts access to confidential documents by disabling clipboard copy, file attachments, and download exports inside AI tool interfaces.",
          ASSETS_DIR / "ui_setup.png",
          "Figure 12.1: Gateway Administration console enforcing dataset isolation, SSL certificates, and tenant domain locking."),
         ("Anomalous AI Data Access Pattern Monitoring", "ACCESS PATTERN SPECIFICATION",
          "The solution should monitor and detect anomalous AI data access patterns that may indicate misuse or compromise.",
          "Tracks token velocity, prompt cadence, and behavioral deviations to detect credential compromise and automated prompt harvesting.",
          ASSETS_DIR / "ui_guard_insights.png",
          "Figure 12.2: Guard Insights anomalous activity monitor tracking real-time policy breach spikes and threat trends.")),

        # Page 15: Requirements 58 & 59
        ("Suspicious Data & Exfiltration Alerts",
         ("Suspicious Data Interaction Detection", "INTERACTION DETECTION SPECIFICATION",
          "The solution must detect abnormal or suspicious interactions between AI systems and enterprise data sources.",
          "Inspects interactions passing between users and external models, flagging abnormal query structures and classified file queries.",
          ASSETS_DIR / "ui_guard_inspect_modal.png",
          "Figure 13.1: Suspicious Interaction Inspector dialog providing deep packet payload analysis and rule evaluation history."),
         ("Automated Data Exfiltration Alerting", "EXFILTRATION ALERT SPECIFICATION",
          "The solution should generate alerts for potential data exfiltration attempts through AI applications.",
          "Instantly triggers browser popups, telemetry alerts, and SOC incident notifications when bulk data extraction is detected.",
          ASSETS_DIR / "extra_img_2.png",
          "Figure 13.2: High-priority security incident console logging automated exfiltration triggers and containment actions.")),

        # Page 16: Requirements 60 & 61
        ("Scraping Mitigation & Workflows",
         ("Abnormal Usage & Automated Scraping Mitigation", "SCRAPING DEFENSE SPECIFICATION",
          "The solution must detect abnormal AI usage patterns such as excessive prompts, automated extraction attempts, or data scraping behaviour.",
          "Rate-limits rapid query generation, terminates automated bot sessions, and logs enumeration sequences at the proxy gateway.",
          ASSETS_DIR / "extra_img_1.png",
          "Figure 14.1: LLM Gateway policy engine enforcing rate limits, token quotas, and prompt scraping defense."),
         ("Security Analyst Investigation Workflows", "ANALYST WORKFLOW SPECIFICATION",
          "The solution should support investigation workflows for security analysts to review AI-related security events.",
          "Equips security analysts with granular search, domain filters, status filters (Allowed vs. Blocked), and complete prompt payload inspection.",
          ASSETS_DIR / "ui_search_logs.png",
          "Figure 14.2: Security Analyst Search & Investigation console with granular search queries, engine filters, and threat scores.")),

        # Page 17: Requirements 62 & 63
        ("Audit Logging & Governance Controls",
         ("Detailed Immutable AI Audit Logging", "AUDIT LOGGING SPECIFICATION",
          "The solution must maintain detailed audit logs of AI activity and data access events.",
          "Captures immutable records detailing date, time, endpoint hostname, AI platform, user prompt, token usage, cost, and exact enforcement decision.",
          ASSETS_DIR / "p5_img2.jpeg",
          "Figure 15.1: Comprehensive immutable audit log table detailing timestamps, user sessions, prompt risk, and verdicts."),
         ("Policy-Aligned Enterprise Governance Controls", "GOVERNANCE CONTROL SPECIFICATION",
          "The solution must provide governance controls for enterprise AI usage aligned with organizational security policies.",
          "Enforces corporate workspace domain authorization (@yespanchi.com), enterprise license quotas, and centralized workstation sync.",
          ASSETS_DIR / "extra_img_4.png",
          "Figure 15.2: Enterprise Governance & Compliance configuration console aligning AI tool permissions with corporate security policies.")),

        # Page 18: Requirements 64 & 65
        ("Compliance Reports & Risk Dashboards",
         ("AI Data Access Compliance Reporting", "COMPLIANCE REPORTING SPECIFICATION",
          "The solution should generate compliance reports related to AI data access and AI system usage.",
          "Provides dynamic executive compliance telemetry capturing total prompts intercepted, high-risk flags, and real-time report export options.",
          ASSETS_DIR / "extra_img_3.png",
          "Figure 16.1: MCP Gateway tools & compliance telemetry dashboard tracking connector requests and export summaries."),
         ("Enterprise AI Risk Posture Dashboards", "RISK POSTURE SPECIFICATION",
          "The solution must provide dashboards for security teams showing AI risk posture across the enterprise.",
          "Displays fleet-wide endpoint agent health, policy interception hit counts (Allowed vs. Blocked), and active workstation risk status.",
          ASSETS_DIR / "ui_overview.png",
          "Figure 16.2: Executive AI Risk Posture overview dashboard displaying daily active agents, intercepted prompts, and threat scores.")),

        # Page 19: Requirements 66 & 67
        ("Audit Reporting & Dataset Protection",
         ("Regulatory & Internal Audit Reporting", "AUDIT REPORTING SPECIFICATION",
          "The solution should support audit reporting for regulatory or internal governance requirements.",
          "Captures forensic compliance trails across web and search queries, logging browser engines, destination result clicks, and threat risk ratings.",
          ASSETS_DIR / "ui_search_logs.png",
          "Figure 17.1: Enterprise Search Activity & Governance Audit log detailing query text, destination URLs, and threat ratings."),
         ("Restricted & Classified Dataset Access Prevention", "DATASET PROTECTION SPECIFICATION",
          "The solution must enable policy enforcement to prevent AI tools from accessing restricted or classified datasets.",
          "Enforces strict prevention policies blocking upload, copy-paste, and drag-and-drop of restricted source files into AI interfaces.",
          EXTRACTED_DIR / "04_image25.png",
          "Figure 17.2: Connected repository permissions console preventing unauthorized AI tool ingestion of sensitive file stores.")),

        # Page 20: Requirements 68 & 69
        ("Runtime Monitoring & Threat Defense",
         ("Runtime Monitoring of AI Interactions", "RUNTIME MONITORING SPECIFICATION",
          "The solution must support runtime monitoring of AI interactions with enterprise data.",
          "Inline proxy broker inspects live data exchanges, evaluating prompt tokens, response safety, and model actions in sub-5ms latency.",
          ASSETS_DIR / "ui_setup.png",
          "Figure 18.1: Gateway Runtime Proxy architecture console configuring real-time data inspection and TLS termination."),
         ("Prompt Injection & Adversarial Attack Detection", "THREAT DEFENSE SPECIFICATION",
          "The solution should detect prompt-based attacks such as prompt injection or manipulation attempts.",
          "Identifies adversarial syntax, recursive prompt injection strings, and instruction overrides, dropping hostile payloads at the socket layer.",
          ASSETS_DIR / "p2_img1.jpeg",
          "Figure 18.2: Real-time defense log intercepting prompt injection payloads, system overrides, and adversarial exploits.")),

        # Page 21: Requirements 70 & 71
        ("Output Protection & Remediation",
         ("Sensitive Output Leakage Prevention", "OUTPUT PROTECTION SPECIFICATION",
          "The solution must prevent AI systems from exposing sensitive information through responses or outputs.",
          "Bidirectional output filter scrubs sensitive tokens, PII, and credentials from downstream model completions before rendering in the browser.",
          ASSETS_DIR / "p4_img2.jpeg",
          "Figure 19.1: Response Scanning dashboard tagging safety classifications and masking leaked output tokens."),
         ("Automated Remediation Actions", "AUTOMATED REMEDIATION SPECIFICATION",
          "The solution should support automated remediation actions such as blocking AI requests, restricting access, or generating alerts.",
          "Automatically executes inline request blocking, drops active TCP sessions, and triggers instant telemetry alerts upon critical policy breaches.",
          ASSETS_DIR / "p3_img1.jpeg",
          "Figure 19.2: Browser Guard enforcement console displaying active automated blocking rules for unauthorized actions.")),

        # Page 22: Requirements 72 & 73
        ("Multi-Asset Risk & Adoption Trends",
         ("Centralized Multi-Asset Risk Exposure Dashboards", "RISK EXPOSURE SPECIFICATION",
          "The solution must provide centralized dashboards displaying AI risk exposure across users, systems, and data assets.",
          "Inspects individual endpoint workstations, detailing hardware identity, physical MAC, OS version, and granular telemetry audit logs.",
          ASSETS_DIR / "extra_img_5.png",
          "Figure 20.1: Multi-Asset Hardware & Workstation telemetry monitor tracking CPU, memory, OS versions, and agent health."),
         ("AI Adoption Trends & Risk Indicator Analytics", "ADOPTION ANALYTICS SPECIFICATION",
          "The solution should provide analytics on AI adoption trends, usage patterns, and risk indicators.",
          "Analyzes query volume growth, cost per user, token consumption, and risk indicator trends across commercial and internal AI platforms.",
          ASSETS_DIR / "ui_overview.png",
          "Figure 20.2: Enterprise AI Adoption trends console mapping active daily users, prompt distribution, and model consumption.")),

        # Page 23: Requirements 74 & 75
        ("Behavioral Scoring & Executive Reports",
         ("Behavioral & Contextual AI Risk Scoring", "BEHAVIORAL SCORING SPECIFICATION",
          "The solution must provide risk scoring for AI activities based on data sensitivity and user behaviour.",
          "Evaluates contextual risk dynamically using historical user behavior, query intent, and search privacy modes.",
          ASSETS_DIR / "p5_img1.jpeg",
          "Figure 21.1: Multi-factor contextual risk matrix weighting user behavior, data classification, and platform security."),
         ("Executive Leadership & Risk Committee Reporting", "EXECUTIVE REPORTING SPECIFICATION",
          "The solution should support executive reporting for security leadership and risk management teams.",
          "Summarizes enterprise AI compliance posture, blocked data leakage attempts, and cost telemetry into executive-level summaries.",
          ASSETS_DIR / "extra_img_4.png",
          "Figure 21.2: Executive Compliance & Governance summary report designed for enterprise risk committee review.")),
    ]

    for page_title, item1, item2 in pairs:
        add_paired_page(story, page_title, item1, item2, img_w=430, img_h=204)

    # Build Document
    print(f"Building document to: {OUTPUT_PDF}")
    doc = SimpleDocTemplate(
        str(OUTPUT_PDF),
        pagesize=A4,
        leftMargin=36,
        rightMargin=36,
        topMargin=46,
        bottomMargin=46,
        title="YP-Raksha InfoSec Compliance Verification Report",
        author="YES PANCHI",
    )
    doc.build(story, canvasmaker=NumberedCanvas)
    print("PDF build complete!")

    # Also save local copies
    import shutil
    shutil.copyfile(str(OUTPUT_PDF), str(LOCAL_COPY))
    print(f"Local copy saved to: {LOCAL_COPY}")
    
    root_copy = BASE_DIR / "YP-Raksha_v1.5_Updated.pdf"
    shutil.copyfile(str(OUTPUT_PDF), str(root_copy))
    print(f"Root copy saved to: {root_copy}")

    root_new_doc = BASE_DIR / "new doc.pdf"
    shutil.copyfile(str(OUTPUT_PDF), str(root_new_doc))
    print(f"Root copy saved to: {root_new_doc}")

if __name__ == "__main__":
    build_pdf()
