import os
from pathlib import Path
from reportlab.lib.pagesizes import A4
from reportlab.lib import colors
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Image, Table, TableStyle, PageBreak
)
from reportlab.pdfgen import canvas
import fitz
import docx
from docx.shared import Inches, Pt, RGBColor
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.enum.table import WD_TABLE_ALIGNMENT
from docx.oxml import parse_xml
from docx.oxml.ns import nsdecls

BASE_DIR = Path(r"d:\unifai_project")
DOCS_DIR = BASE_DIR / "docs"
LOGO_PATH = str(BASE_DIR / "ui" / "public" / "yes-panchi-logo.png")
PDF_OUTPUT = DOCS_DIR / "YP_Raksha_Technical_Documentation_v2.5.pdf"
DOCX_OUTPUT = DOCS_DIR / "YP_Raksha_Technical_Documentation_v2.5.docx"
MD_OUTPUT = DOCS_DIR / "YP_Raksha_Technical_Documentation_v2.5.md"

PAGE_W, PAGE_H = A4  # 595.27 x 841.89 pt
MARGIN = 36          # 0.5 inch margins
CONTENT_W = PAGE_W - (MARGIN * 2)  # 523.27 pt

# Colors
NAVY = colors.HexColor("#0F172A")        # Slate 900
SLATE_DARK = colors.HexColor("#1E293B")  # Slate 800
SLATE_BODY = colors.HexColor("#334155")  # Slate 700
MUTED = colors.HexColor("#64748B")       # Slate 500
BORDER_COLOR = colors.HexColor("#CBD5E1")# Slate 300
BORDER_LIGHT = colors.HexColor("#E2E8F0")# Slate 200
GREEN = colors.HexColor("#059669")       # Emerald 600
GREEN_LIGHT = colors.HexColor("#F0FDF4") # Emerald 50
GREEN_BORDER = colors.HexColor("#86EFAC")# Emerald 300
TEAL = colors.HexColor("#0F766E")        # Teal 700
TEAL_BG = colors.HexColor("#F0FDFA")     # Teal 50
CARD_BG = colors.HexColor("#F8FAFC")     # Slate 50
WHITE = colors.white

def get_pdf_styles():
    return {
        # Cover Styles
        "cover_company": ParagraphStyle("cover_company", fontName="Helvetica-Bold", fontSize=15, leading=19, textColor=NAVY, alignment=1, spaceAfter=4),
        "cover_product": ParagraphStyle("cover_product", fontName="Helvetica-Bold", fontSize=42, leading=48, textColor=NAVY, alignment=1, spaceAfter=3),
        "cover_tagline": ParagraphStyle("cover_tagline", fontName="Helvetica-Bold", fontSize=10.5, leading=14, textColor=GREEN, alignment=1, spaceAfter=14),
        "cover_title": ParagraphStyle("cover_title", fontName="Helvetica-Bold", fontSize=23, leading=29, textColor=SLATE_DARK, alignment=1, spaceAfter=4),
        "cover_version": ParagraphStyle("cover_version", fontName="Helvetica-Bold", fontSize=13, leading=17, textColor=MUTED, alignment=1, spaceAfter=12),
        "cover_desc": ParagraphStyle("cover_desc", fontName="Helvetica", fontSize=9.2, leading=13.5, textColor=SLATE_BODY, alignment=1, spaceAfter=16),
        "cover_copy": ParagraphStyle("cover_copy", fontName="Helvetica-Oblique", fontSize=7.2, leading=10, textColor=MUTED, alignment=1),
        "meta_key": ParagraphStyle("meta_key", fontName="Helvetica-Bold", fontSize=7.8, leading=10.5, textColor=NAVY),
        "meta_val": ParagraphStyle("meta_val", fontName="Helvetica", fontSize=7.8, leading=10.5, textColor=SLATE_BODY),

        # Table of Contents Styles
        "toc_title": ParagraphStyle("toc_title", fontName="Helvetica-Bold", fontSize=18, leading=22, textColor=NAVY, alignment=1, spaceAfter=3),
        "toc_sub": ParagraphStyle("toc_sub", fontName="Helvetica", fontSize=8.5, leading=12, textColor=MUTED, alignment=1, spaceAfter=12),
        "toc_th": ParagraphStyle("toc_th", fontName="Helvetica-Bold", fontSize=7.8, leading=10.5, textColor=WHITE),
        "toc_th_c": ParagraphStyle("toc_th_c", fontName="Helvetica-Bold", fontSize=7.8, leading=10.5, textColor=WHITE, alignment=1),
        "toc_sec": ParagraphStyle("toc_sec", fontName="Helvetica-Bold", fontSize=7.4, leading=9.5, textColor=NAVY, alignment=1),
        "toc_name": ParagraphStyle("toc_name", fontName="Helvetica-Bold", fontSize=7.4, leading=9.5, textColor=SLATE_DARK),
        "toc_desc": ParagraphStyle("toc_desc", fontName="Helvetica", fontSize=6.5, leading=8.5, textColor=MUTED),
        "toc_page": ParagraphStyle("toc_page", fontName="Helvetica-Bold", fontSize=7.4, leading=9.5, textColor=GREEN, alignment=1),

        # Headings & Section Styles
        "h1": ParagraphStyle("h1", fontName="Helvetica-Bold", fontSize=14, leading=18, textColor=NAVY, spaceAfter=5),
        "h2": ParagraphStyle("h2", fontName="Helvetica-Bold", fontSize=9.5, leading=13, textColor=TEAL, spaceBefore=6, spaceAfter=3),
        "sec_head": ParagraphStyle("sec_head", fontName="Helvetica-Bold", fontSize=8.2, leading=11, textColor=NAVY, spaceBefore=4, spaceAfter=2),
        "body": ParagraphStyle("body", fontName="Helvetica", fontSize=8.0, leading=11.2, textColor=SLATE_BODY, spaceAfter=4),
        "bullet": ParagraphStyle("bullet", fontName="Helvetica", fontSize=7.6, leading=10.6, textColor=SLATE_BODY, spaceAfter=2.5, leftIndent=8),
        "grid_bullet": ParagraphStyle("grid_bullet", fontName="Helvetica", fontSize=7.2, leading=9.8, textColor=SLATE_BODY, spaceAfter=2, leftIndent=6),
        "note": ParagraphStyle("note", fontName="Helvetica-Oblique", fontSize=6.8, leading=9.2, textColor=MUTED, spaceBefore=3, spaceAfter=2),

        # Card Styles (for 2-Column Tables)
        "card_title": ParagraphStyle("card_title", fontName="Helvetica-Bold", fontSize=7.6, leading=9.8, textColor=NAVY, spaceAfter=2),
        "card_body": ParagraphStyle("card_body", fontName="Helvetica", fontSize=7.0, leading=9.4, textColor=SLATE_BODY),
        "card_body_uc": ParagraphStyle("card_body_uc", fontName="Helvetica", fontSize=7.0, leading=9.4, textColor=SLATE_BODY, spaceAfter=2),
        "card_benefit": ParagraphStyle("card_benefit", fontName="Helvetica", fontSize=7.0, leading=9.4, textColor=GREEN),
        "callout_text": ParagraphStyle("callout_text", fontName="Helvetica", fontSize=7.0, leading=9.6, textColor=SLATE_DARK),
    }

class NumberedCanvas(canvas.Canvas):
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
                self.drawImage(LOGO_PATH, MARGIN, PAGE_H - 32, width=60, height=19, preserveAspectRatio=True, mask='auto')
            except Exception:
                self.setFont("Helvetica-Bold", 8)
                self.setFillColor(NAVY)
                self.drawString(MARGIN, PAGE_H - 25, "YES PANCHI")

            self.setFont("Helvetica-Bold", 7.5)
            self.setFillColor(SLATE_DARK)
            self.drawRightString(PAGE_W - MARGIN, PAGE_H - 21, "YP-Raksha v2.5  |  Technical Documentation")
            
            self.setFont("Helvetica", 6.8)
            self.setFillColor(MUTED)
            self.drawRightString(PAGE_W - MARGIN, PAGE_H - 29, "Enterprise AI Security, Real-time Knowledge Screening & Hazard Audit")

            self.setStrokeColor(BORDER_COLOR)
            self.setLineWidth(0.6)
            self.line(MARGIN, PAGE_H - 35, PAGE_W - MARGIN, PAGE_H - 35)

            # Running Footer (Pages 2 through total_pages)
            self.setStrokeColor(BORDER_COLOR)
            self.setLineWidth(0.6)
            self.line(MARGIN, 34, PAGE_W - MARGIN, 34)

            self.setFont("Helvetica-Bold", 7.2)
            self.setFillColor(NAVY)
            self.drawString(MARGIN, 22, "YES PANCHI GROUP OF COMPANIES")
            
            self.setFont("Helvetica", 7.0)
            self.setFillColor(MUTED)
            self.drawString(185, 22, "CONFIDENTIAL & PROPRIETARY — FOR AUTHORIZED AUDIT ONLY")

            self.setFont("Helvetica-Bold", 7.2)
            self.setFillColor(GREEN)
            self.drawRightString(PAGE_W - MARGIN, 22, f"Page {page_num} of {total_pages}")

        self.restoreState()

def build_pdf():
    ST = get_pdf_styles()
    story = []

    # ==========================================
    # PAGE 1: TITLE / COVER PAGE (PERFECTLY BALANCED)
    # ==========================================
    story.append(Spacer(1, 45))
    try:
        logo_img = Image(LOGO_PATH, width=230, height=73)
        logo_img.hAlign = 'CENTER'
        story.append(logo_img)
    except Exception as e:
        story.append(Paragraph("<b>YES PANCHI</b>", ST["cover_company"]))
    
    story.append(Spacer(1, 20))
    story.append(Paragraph("YES PANCHI", ST["cover_company"]))
    story.append(Paragraph("YP-RAKSHA", ST["cover_product"]))
    story.append(Paragraph("RAKSHA — REAL-TIME AI KNOWLEDGE SCREENING & HAZARD AUDIT", ST["cover_tagline"]))
    story.append(Spacer(1, 14))
    story.append(Paragraph("Technical Documentation", ST["cover_title"]))
    story.append(Paragraph("Version 2.5", ST["cover_version"]))
    story.append(Paragraph(
        "Complete enterprise architectural blueprint, endpoint security engine, proxy interception mechanics, "
        "unified LLM gateway fabric, and compliance governance specification.",
        ST["cover_desc"]
    ))
    story.append(Spacer(1, 26))

    # Metadata Table (Balanced Vertical Height)
    meta_data = [
        [Paragraph("Product Name", ST["meta_key"]), Paragraph("YP-Raksha (YesPanchi Raksha AI Security Platform)", ST["meta_val"])],
        [Paragraph("Document Type", ST["meta_key"]), Paragraph("Enterprise Architecture & Technical Specification Manual", ST["meta_val"])],
        [Paragraph("System Version", ST["meta_key"]), Paragraph("Version 2.5 (Gateway Core Go 1.26.4 / Python 3.11+ mitmproxy Guard Agent 1.1.20)", ST["meta_val"])],
        [Paragraph("Security Classification", ST["meta_key"]), Paragraph("Confidential — Enterprise Internal & Partner Technical Audit", ST["meta_val"])],
        [Paragraph("Target Audience", ST["meta_key"]), Paragraph("Chief Information Security Officers (CISO), Enterprise Architects, SecOps, InfoSec Compliance Auditors", ST["meta_val"])],
        [Paragraph("Publishing Organization", ST["meta_key"]), Paragraph("YesPanchi Group of Companies — Cyber Defense & Enterprise AI Division", ST["meta_val"])],
        [Paragraph("Release Date", ST["meta_key"]), Paragraph("October 2026", ST["meta_val"])],
        [Paragraph("Document Status", ST["meta_key"]), Paragraph("Approved Enterprise Baseline Technical Specification", ST["meta_val"])],
    ]
    meta_table = Table(meta_data, colWidths=[130, 393.27])
    meta_table.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,-1), CARD_BG),
        ('BOX', (0,0), (-1,-1), 0.6, BORDER_COLOR),
        ('INNERGRID', (0,0), (-1,-1), 0.4, BORDER_LIGHT),
        ('TOPPADDING', (0,0), (-1,-1), 6.0),
        ('BOTTOMPADDING', (0,0), (-1,-1), 6.0),
        ('LEFTPADDING', (0,0), (-1,-1), 8),
        ('RIGHTPADDING', (0,0), (-1,-1), 8),
        ('VALIGN', (0,0), (-1,-1), 'MIDDLE'),
        ('ROWBACKGROUNDS', (0,0), (-1,-1), [WHITE, CARD_BG]),
    ]))
    story.append(meta_table)
    story.append(Spacer(1, 30))
    story.append(Paragraph("© 2026 YesPanchi Group of Companies. All rights reserved. Proprietary and Confidential.", ST["cover_copy"]))

    # ==========================================
    # PAGE 2: FULL INDEX (TABLE OF CONTENTS)
    # ==========================================
    story.append(PageBreak())
    story.append(Spacer(1, 4))
    story.append(Paragraph("Table of Contents", ST["toc_title"]))
    story.append(Paragraph("YP-Raksha v2.5 — Complete Enterprise Technical Architecture Index", ST["toc_sub"]))

    toc_headers = [
        Paragraph("Sec", ST["toc_th_c"]),
        Paragraph("Module & Topic Specification", ST["toc_th"]),
        Paragraph("Detailed Engineering Subsystems & Topics", ST["toc_th"]),
        Paragraph("Page", ST["toc_th_c"])
    ]
    
    toc_items = [
        ("1.0", "Product Overview & Mission", "1.1 Intro to Raksha | 1.2 Purpose & Objectives | 1.3 Key Features | 1.4 Use Cases & Benefits | 1.5 Scope & Limitations", "Page 3 - 4"),
        ("2.0", "System Architecture & High-Level Design", "2.1 Dual Data Planes | 2.2 Ingestion & Proxy Pipelines | 2.3 Persistence & Multi-Storage Fabric | 2.4 Cryptographic Boundaries", "Page 5"),
        ("3.0", "Browser Guard & Endpoint Fleet Mechanics", "3.1 Python mitmproxy Agent | 3.2 PAC Steering (18103/18195) | 3.3 Dynamic Content Parsing | 3.4 Triple Remediation Pipeline", "Page 6"),
        ("4.0", "Unified AI Gateway & Provider Fabric", "4.1 OpenAI-Compatible /v1/* Modality Engine | 4.2 28+ Provider Adapters | 4.3 Failover & Load Balancing | 4.4 Model Context Protocol", "Page 7"),
        ("5.0", "Governance, Budgets & Virtual Keys", "5.1 Cryptographic Virtual Keys | 5.2 Multi-Tier Budget Quotas | 5.3 TPM/RPM Throttling | 5.4 Chargebacks & Cost Attribution", "Page 8"),
        ("6.0", "Security Enforcement & Hazard Screening", "6.1 Inline PII & Regulated Data Masking | 6.2 CEL Policy Rules | 6.3 Secondary AI Guard Bot | 6.4 Jailbreak & Prompt Injection Defense", "Page 9"),
        ("7.0", "Identity Federation & Enterprise RBAC", "7.1 Multi-Tenant RBAC Matrix | 7.2 SCIM 2.0 Inbound Sync (Okta/Entra) | 7.3 Active Directory LDAP/LDAPS | 7.4 Session Security Policies", "Page 10"),
        ("8.0", "Observability, Telemetry & Audit Trails", "8.1 Real-Time Prompt/Search Logs | 8.2 OTel & Prometheus Exporters | 8.3 Immutable Audit Trails | 8.4 CISO Analytics Dashboard", "Page 11"),
        ("9.0", "Enterprise Deployment & Infrastructure", "9.1 Containerized Stack (Docker & Helm) | 9.2 Endpoint Packages (Inno Setup / Mac) | 9.3 Secrets Hardening | 9.4 High Availability", "Page 12"),
        ("10.0", "Technical Glossary & Reference Matrix", "10.1 Key Terminology & Definitions | 10.2 Network Port & Protocol Matrix | 10.3 Failover Protocol & Diagnostic Runbook", "Page 13"),
    ]

    toc_table_data = [toc_headers]
    for sec, title, desc, pg in toc_items:
        toc_table_data.append([
            Paragraph(sec, ST["toc_sec"]),
            Paragraph(title, ST["toc_name"]),
            Paragraph(desc, ST["toc_desc"]),
            Paragraph(pg, ST["toc_page"])
        ])

    toc_table = Table(toc_table_data, colWidths=[32, 150, 281.27, 60])
    toc_table.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,0), NAVY),
        ('BOX', (0,0), (-1,-1), 0.6, BORDER_COLOR),
        ('INNERGRID', (0,0), (-1,-1), 0.4, BORDER_LIGHT),
        ('TOPPADDING', (0,0), (-1,-1), 7.5),
        ('BOTTOMPADDING', (0,0), (-1,-1), 7.5),
        ('LEFTPADDING', (0,0), (-1,-1), 6),
        ('RIGHTPADDING', (0,0), (-1,-1), 6),
        ('VALIGN', (0,0), (-1,-1), 'MIDDLE'),
        ('ROWBACKGROUNDS', (0,1), (-1,-1), [WHITE, CARD_BG]),
    ]))
    story.append(toc_table)
    
    story.append(Spacer(1, 16))
    
    # Document Roadmap Callout Box
    roadmap_p = Paragraph(
        "<b>Architectural Scope Note:</b> This technical specification is organized modularly to provide complete "
        "verification depth across all operational layers of YP-Raksha v2.5. Each section delivers verified implementation facts from source code, "
        "operational binaries, and configuration schemas, serving as the official technical baseline for deployment and enterprise infosec compliance.",
        ST["callout_text"]
    )
    roadmap_box = Table([[roadmap_p]], colWidths=[CONTENT_W])
    roadmap_box.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,-1), TEAL_BG),
        ('BOX', (0,0), (-1,-1), 0.6, colors.HexColor("#99F6E4")),
        ('LINELEFT', (0,0), (0,-1), 2.5, TEAL),
        ('TOPPADDING', (0,0), (-1,-1), 8),
        ('BOTTOMPADDING', (0,0), (-1,-1), 8),
        ('LEFTPADDING', (0,0), (-1,-1), 10),
        ('RIGHTPADDING', (0,0), (-1,-1), 10),
    ]))
    story.append(roadmap_box)

    # ==========================================
    # PAGE 3: 1. PRODUCT OVERVIEW (PART 1: 1.1, 1.2, 1.3)
    # ==========================================
    story.append(PageBreak())
    story.append(Spacer(1, 1))
    story.append(Paragraph("1. Product Overview", ST["h1"]))

    # 1.1 Introduction to Raksha AI Security
    story.append(Paragraph("1.1 Introduction to Raksha AI Security", ST["h2"]))
    p1_1_items = [
        "Raksha AI Security is an enterprise-grade AI security and governance solution developed by YesPanchi to help organizations monitor, control, and secure the use of Artificial Intelligence (AI) applications and services.",
        "As organizations increasingly adopt AI tools such as ChatGPT, Claude, Gemini, DeepSeek, and other AI-powered platforms, protecting sensitive business information becomes essential. Employees may unintentionally submit confidential data, customer information, internal documents, or other sensitive content to AI applications.",
        "Raksha AI Security helps address these risks by providing centralized monitoring, security policy enforcement, data loss prevention (DLP), and AI usage governance. It enables organizations to establish security controls over AI interactions while maintaining visibility into how AI applications are used across the enterprise.",
        "The solution is designed to support enterprise security requirements, including controlled AI access, sensitive-data protection, risk monitoring, and administrative oversight. It can be deployed according to an organization's infrastructure and security requirements, including on-premises deployment where supported and configured."
    ]
    for text in p1_1_items:
        story.append(Paragraph(text, ST["body"]))

    # 1.2 Product Purpose and Objectives
    story.append(Paragraph("1.2 Product Purpose and Objectives", ST["h2"]))
    story.append(Paragraph("The primary purpose of Raksha AI Security is to enable organizations to use AI technologies securely while reducing the risk of confidential information exposure, unauthorized AI usage, and policy violations.", ST["body"]))
    story.append(Paragraph("The key objectives are:", ST["body"]))
    objs = [
        ("AI Usage Monitoring", "Provide visibility into AI application usage and relevant interactions across monitored endpoints and supported platforms."),
        ("Data Loss Prevention (DLP)", "Identify and help prevent sensitive information, such as confidential business data and personally identifiable information, from being submitted to AI applications in violation of configured policies."),
        ("Security Policy Enforcement", "Allow administrators to define and enforce organizational policies for AI application access, usage, and data protection."),
        ("Risk Detection and Prevention", "Identify potentially risky AI interactions and apply configured security actions, such as warnings, blocking, or logging, where supported."),
        ("Centralized Administration", "Provide a centralized interface for managing security configurations, monitoring activity, and reviewing relevant security events."),
        ("AI Governance and Accountability", "Support organizations in establishing consistent controls, maintaining audit visibility, and improving oversight of enterprise AI usage."),
        ("Enterprise Data Protection", "Support the implementation of organizational security and data-residency requirements through appropriate deployment architecture and configuration."),
    ]
    for idx, (title, desc) in enumerate(objs, 1):
        story.append(Paragraph(f"{idx}. <b>{title}:</b> {desc}", ST["bullet"]))

    # 1.3 Key Features and Capabilities (2-Column Card Grid for Perfect Spacing)
    story.append(Paragraph("1.3 Key Features and Capabilities", ST["h2"]))
    story.append(Paragraph("Raksha AI Security provides a set of security and governance capabilities for enterprise AI environments.", ST["body"]))
    feats = [
        ("1. AI Application Monitoring", "Monitors supported AI applications and interactions through configured monitoring components, providing visibility into relevant AI usage and security events."),
        ("2. Real-Time Data Loss Prevention", "Evaluates monitored content against configured security rules to identify sensitive information and apply appropriate protective actions."),
        ("3. Prompt Inspection and Risk Detection", "Analyzes AI prompts and supported interaction data to identify potential security risks, sensitive information, and policy violations."),
        ("4. Policy and Rule Management", "Enables administrators to configure security rules, monitoring policies, risk thresholds, and enforcement settings according to organizational requirements."),
        ("5. Website and Application Controls", "Supports configured website and AI application monitoring and access-control policies for supported environments."),
        ("6. Security Logs and Audit Visibility", "Provides relevant logs and event information to help administrators investigate policy violations, review AI usage, and understand detected risks."),
        ("7. Centralized Dashboard", "Provides a consolidated view of available monitoring data, security events, risk indicators, and system activity to support administrative oversight."),
        ("8. Enterprise Access and Governance", "Supports controlled administration of users, teams, customers, business units, and virtual keys, where applicable to the deployed Gateway configuration."),
        ("9. AI Gateway Governance", "Supports centralized management of AI model providers, routing configurations, usage limits, and budget controls within the Gateway component."),
        ("10. Enterprise Deployment Support", "Supports deployment and configuration according to organizational infrastructure requirements, including on-premises environments where supported."),
    ]
    feat_table_data = []
    for i in range(0, len(feats), 2):
        row = []
        for f_title, f_desc in feats[i:i+2]:
            cell = [Paragraph(f_title, ST["card_title"]), Paragraph(f_desc, ST["card_body"])]
            row.append(cell)
        feat_table_data.append(row)

    t_feat = Table(feat_table_data, colWidths=[CONTENT_W/2, CONTENT_W/2])
    t_feat.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,-1), CARD_BG),
        ('BOX', (0,0), (-1,-1), 0.5, BORDER_LIGHT),
        ('INNERGRID', (0,0), (-1,-1), 0.5, BORDER_LIGHT),
        ('TOPPADDING', (0,0), (-1,-1), 3.5),
        ('BOTTOMPADDING', (0,0), (-1,-1), 3.5),
        ('LEFTPADDING', (0,0), (-1,-1), 6),
        ('RIGHTPADDING', (0,0), (-1,-1), 6),
        ('VALIGN', (0,0), (-1,-1), 'TOP'),
    ]))
    story.append(t_feat)
    story.append(Paragraph("<i>*Note: Actual feature availability, enforcement behavior, supported applications, and deployment options depend on the installed product version, enabled modules, and validated configuration.</i>", ST["note"]))

    # ==========================================
    # PAGE 4: 1. PRODUCT OVERVIEW (PART 2: 1.4, 1.5)
    # ==========================================
    story.append(PageBreak())
    story.append(Spacer(1, 1))

    # 1.4 Use Cases and Business Benefits (2-Column Card Grid for Perfect Spacing)
    story.append(Paragraph("1.4 Use Cases and Business Benefits", ST["h2"]))
    story.append(Paragraph("Raksha AI Security can support a range of enterprise security and governance scenarios.", ST["body"]))
    ucs = [
        ("Use Case 1: Preventing Confidential Data Exposure", "Employees may accidentally enter confidential company information into public AI tools. Raksha can inspect monitored interactions against configured policies and apply the appropriate security response.", "Reduces the risk of unauthorized disclosure of sensitive business information."),
        ("Use Case 2: Enterprise AI Usage Monitoring", "Security and IT teams need visibility into how employees use supported AI applications.", "Improves visibility, security oversight, and understanding of AI usage across the organization."),
        ("Use Case 3: Enforcing Organizational AI Policies", "Organizations may restrict the use of specific AI applications or prohibit certain categories of sensitive information from being submitted to AI services.", "Helps enforce consistent AI usage policies and reduce policy violations."),
        ("Use Case 4: Security Incident Investigation", "When a potential AI-related security incident occurs, administrators need relevant event logs and monitoring information to investigate the incident.", "Supports incident analysis, security reviews, and audit preparation."),
        ("Use Case 5: Enterprise AI Gateway Governance", "Organizations using multiple AI providers and models may need centralized routing, access management, and budget controls.", "Helps improve governance, operational visibility, and control over configured AI usage and expenditure."),
        ("Use Case 6: Regulated and Sensitive Environments", "Organizations such as banks and financial institutions may require strict controls over sensitive information, infrastructure access, and data residency.", "Supports the implementation of internal security and compliance controls when the solution is deployed and validated against the organization's requirements."),
    ]
    uc_table_data = []
    for i in range(0, len(ucs), 2):
        row = []
        for t, d, b in ucs[i:i+2]:
            cell = [
                Paragraph(t, ST["card_title"]),
                Paragraph(d, ST["card_body_uc"]),
                Paragraph(f"<b>Business benefit:</b> {b}", ST["card_benefit"])
            ]
            row.append(cell)
        uc_table_data.append(row)

    t_uc = Table(uc_table_data, colWidths=[CONTENT_W/2, CONTENT_W/2])
    t_uc.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,-1), CARD_BG),
        ('BOX', (0,0), (-1,-1), 0.5, BORDER_LIGHT),
        ('INNERGRID', (0,0), (-1,-1), 0.5, BORDER_LIGHT),
        ('TOPPADDING', (0,0), (-1,-1), 3.5),
        ('BOTTOMPADDING', (0,0), (-1,-1), 3.5),
        ('LEFTPADDING', (0,0), (-1,-1), 6),
        ('RIGHTPADDING', (0,0), (-1,-1), 6),
        ('VALIGN', (0,0), (-1,-1), 'TOP'),
    ]))
    story.append(t_uc)

    story.append(Paragraph("Overall Business Benefits", ST["sec_head"]))
    obs = [
        "Improved visibility into supported AI usage.",
        "Reduced risk of sensitive-data exposure.",
        "More consistent enforcement of AI security policies.",
        "Centralized monitoring and administrative control.",
        "Improved support for security investigations and audits.",
        "Better governance of AI providers, models, and configured budgets.",
        "Support for enterprise security and data-residency requirements.",
    ]
    ob_rows = []
    for i in range(0, len(obs), 2):
        c1 = f"- {obs[i]}"
        c2 = f"- {obs[i+1]}" if i+1 < len(obs) else ""
        ob_rows.append([Paragraph(c1, ST["grid_bullet"]), Paragraph(c2, ST["grid_bullet"])])
    t_ob = Table(ob_rows, colWidths=[CONTENT_W/2, CONTENT_W/2])
    t_ob.setStyle(TableStyle([
        ('TOPPADDING', (0,0), (-1,-1), 1.5),
        ('BOTTOMPADDING', (0,0), (-1,-1), 1.5),
        ('LEFTPADDING', (0,0), (-1,-1), 2),
        ('RIGHTPADDING', (0,0), (-1,-1), 2),
    ]))
    story.append(t_ob)

    # 1.5 Product Scope and Limitations
    story.append(Paragraph("1.5 Product Scope and Limitations", ST["h2"]))
    story.append(Paragraph("<b>Product Scope:</b> Raksha AI Security is intended to provide security monitoring, data protection, and governance capabilities for supported AI applications and configured enterprise AI environments. The product scope includes:", ST["body"]))

    scopes = [
        "Monitoring supported AI applications and interactions.",
        "Inspecting monitored prompts and content against configured security rules.",
        "Detecting potential sensitive-data exposure and policy violations.",
        "Applying supported security actions according to configured policies.",
        "Maintaining relevant security logs and monitoring information.",
        "Providing administrative controls and dashboard visibility.",
        "Supporting configured AI Gateway governance, including provider, model, routing, and budget management.",
        "Supporting enterprise deployment architectures based on validated product capabilities and infrastructure requirements.",
    ]
    sc_rows = []
    for i in range(0, len(scopes), 2):
        c1 = f"- {scopes[i]}"
        c2 = f"- {scopes[i+1]}" if i+1 < len(scopes) else ""
        sc_rows.append([Paragraph(c1, ST["grid_bullet"]), Paragraph(c2, ST["grid_bullet"])])
    t_sc = Table(sc_rows, colWidths=[CONTENT_W/2, CONTENT_W/2])
    t_sc.setStyle(TableStyle([
        ('TOPPADDING', (0,0), (-1,-1), 1.5),
        ('BOTTOMPADDING', (0,0), (-1,-1), 1.5),
        ('LEFTPADDING', (0,0), (-1,-1), 2),
        ('RIGHTPADDING', (0,0), (-1,-1), 2),
    ]))
    story.append(t_sc)

    story.append(Paragraph("<b>Product Limitations:</b>", ST["sec_head"]))
    limits = [
        ("Supported Applications", "Monitoring and enforcement depend on the AI applications, websites, browsers, operating systems, and integration methods supported by the deployed version."),
        ("Monitoring Coverage", "Raksha can inspect only the interactions and data accessible to its configured monitoring components. It should not be assumed to monitor every application or every communication channel automatically."),
        ("Detection Accuracy", "Detection performance depends on the configured rules, inspection methods, supported content types, and detection models. False positives and false negatives may occur."),
        ("Endpoint Configuration", "Monitoring and enforcement may depend on successful installation, configuration, permissions, and operation of the relevant endpoint agent, extension, or other monitoring component."),
        ("Network and Security Restrictions", "Firewalls, proxies, antivirus software, endpoint protection tools, and organizational security policies may affect installation, connectivity, or operation."),
        ("Deployment Requirements", "On-premises deployment, high availability, disaster recovery, endpoint scalability, and offline operation must be validated against the actual product architecture and deployment configuration."),
        ("Data Residency", "Keeping all data, logs, and telemetry within a bank's infrastructure or within India requires appropriate architecture, network restrictions, storage configuration, and verification. This must be confirmed through deployment testing and network-flow validation."),
        ("Third-Party AI Services", "When users access external AI services, information submitted to those services may be processed according to the respective provider's policies and infrastructure. Raksha monitoring alone does not guarantee that external providers will not retain or process submitted information."),
        ("Compliance Responsibility", "Raksha can support organizational security and governance processes but does not independently guarantee regulatory compliance or certification."),
        ("Feature Availability", "Certain features may depend on the product edition, enabled modules, software version, licenses, or administrative configuration."),
    ]
    lim_rows = []
    for i in range(0, len(limits), 2):
        row = []
        for idx, (lt, ld) in enumerate(limits[i:i+2], i+1):
            p = Paragraph(f"{idx}. <b>{lt}:</b> {ld}", ST["grid_bullet"])
            row.append(p)
        if len(row) == 1:
            row.append(Paragraph("", ST["grid_bullet"]))
        lim_rows.append(row)

    t_lim = Table(lim_rows, colWidths=[CONTENT_W/2, CONTENT_W/2])
    t_lim.setStyle(TableStyle([
        ('TOPPADDING', (0,0), (-1,-1), 1.5),
        ('BOTTOMPADDING', (0,0), (-1,-1), 1.5),
        ('LEFTPADDING', (0,0), (-1,-1), 2),
        ('RIGHTPADDING', (0,0), (-1,-1), 2),
    ]))
    story.append(t_lim)
    story.append(Paragraph("<i>The deployment team should validate the supported environments, security controls, data flows, performance, and recovery procedures before production rollout.</i>", ST["note"]))

    # Build Document
    doc = SimpleDocTemplate(
        str(PDF_OUTPUT),
        pagesize=A4,
        leftMargin=MARGIN,
        rightMargin=MARGIN,
        topMargin=MARGIN,
        bottomMargin=MARGIN
    )
    doc.build(story, canvasmaker=NumberedCanvas)
    print(f"Built PDF: {PDF_OUTPUT}")

def build_docx():
    doc = docx.Document()
    
    # Configure A4 Margins
    for sec in doc.sections:
        sec.page_width = Inches(8.27)
        sec.page_height = Inches(11.69)
        sec.top_margin = Inches(0.6)
        sec.bottom_margin = Inches(0.6)
        sec.left_margin = Inches(0.6)
        sec.right_margin = Inches(0.6)

    NAVY_HEX = "0F172A"
    NAVY_RGB = RGBColor(0x0F, 0x17, 0x2A)
    GREEN_RGB = RGBColor(0x05, 0x96, 0x69)
    MUTED_RGB = RGBColor(0x64, 0x74, 0x8B)
    SLATE_BODY_RGB = RGBColor(0x33, 0x41, 0x55)
    TEAL_RGB = RGBColor(0x0F, 0x76, 0x6E)

    def set_font(run, size=11, bold=False, italic=False, color_rgb=None, font_name="Calibri"):
        run.font.name = font_name
        run.font.size = Pt(size)
        run.bold = bold
        run.italic = italic
        if color_rgb:
            run.font.color.rgb = color_rgb

    def set_cell_background(cell, fill_hex):
        tcPr = cell._tc.get_or_add_tcPr()
        shd = parse_xml(f'<w:shd {nsdecls("w")} w:fill="{fill_hex}"/>')
        tcPr.append(shd)

    def set_cell_margins(cell, top=60, bottom=60, left=100, right=100):
        tcPr = cell._tc.get_or_add_tcPr()
        tcMar = parse_xml(
            f'<w:tcMar {nsdecls("w")}>'
            f'<w:top w:w="{top}" w:type="dxa"/>'
            f'<w:bottom w:w="{bottom}" w:type="dxa"/>'
            f'<w:left w:w="{left}" w:type="dxa"/>'
            f'<w:right w:w="{right}" w:type="dxa"/>'
            f'</w:tcMar>'
        )
        tcPr.append(tcMar)

    # ---------------- PAGE 1: COVER ----------------
    for _ in range(3):
        p_empty = doc.add_paragraph()
        p_empty.paragraph_format.space_after = Pt(0)

    # Add Logo
    if Path(LOGO_PATH).exists():
        p_logo = doc.add_paragraph()
        p_logo.alignment = WD_ALIGN_PARAGRAPH.CENTER
        p_logo.paragraph_format.space_after = Pt(16)
        run_logo = p_logo.add_run()
        run_logo.add_picture(LOGO_PATH, width=Inches(3.2))

    p_comp = doc.add_paragraph()
    p_comp.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_comp.paragraph_format.space_after = Pt(2)
    r = p_comp.add_run("YES PANCHI")
    set_font(r, size=16, bold=True, color_rgb=NAVY_RGB)

    p_prod = doc.add_paragraph()
    p_prod.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_prod.paragraph_format.space_after = Pt(2)
    r = p_prod.add_run("YP-RAKSHA")
    set_font(r, size=36, bold=True, color_rgb=NAVY_RGB)

    p_sub = doc.add_paragraph()
    p_sub.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_sub.paragraph_format.space_after = Pt(8)
    r = p_sub.add_run("RAKSHA — REAL-TIME AI KNOWLEDGE SCREENING & HAZARD AUDIT")
    set_font(r, size=10, bold=True, color_rgb=GREEN_RGB)

    p_title = doc.add_paragraph()
    p_title.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_title.paragraph_format.space_after = Pt(2)
    r = p_title.add_run("Technical Documentation")
    set_font(r, size=24, bold=True, color_rgb=NAVY_RGB)

    p_ver = doc.add_paragraph()
    p_ver.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_ver.paragraph_format.space_after = Pt(14)
    r = p_ver.add_run("Version 2.5")
    set_font(r, size=13, bold=True, color_rgb=MUTED_RGB)

    p_desc = doc.add_paragraph()
    p_desc.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_desc.paragraph_format.space_after = Pt(24)
    r = p_desc.add_run(
        "Complete enterprise architectural blueprint, endpoint security engine, proxy interception mechanics, "
        "unified LLM gateway fabric, and compliance governance specification."
    )
    set_font(r, size=9.5, italic=True, color_rgb=SLATE_BODY_RGB)

    # Metadata Table
    meta_items = [
        ("Product Name", "YP-Raksha (YesPanchi Raksha AI Security Platform)"),
        ("Document Type", "Enterprise Architecture & Technical Specification Manual"),
        ("System Version", "Version 2.5 (Gateway Core Go 1.26.4 / Python 3.11+ mitmproxy Guard Agent 1.1.20)"),
        ("Security Classification", "Confidential — Enterprise Internal & Partner Technical Audit"),
        ("Target Audience", "Chief Information Security Officers (CISO), Enterprise Architects, SecOps, InfoSec Compliance Auditors"),
        ("Publishing Organization", "YesPanchi Group of Companies — Cyber Defense & Enterprise AI Division"),
        ("Release Date", "October 2026"),
        ("Document Status", "Approved Enterprise Baseline Technical Specification"),
    ]
    t_meta = doc.add_table(rows=len(meta_items), cols=2)
    t_meta.alignment = WD_TABLE_ALIGNMENT.CENTER
    for idx, (k, v) in enumerate(meta_items):
        c1, c2 = t_meta.rows[idx].cells
        c1.width = Inches(2.0)
        c2.width = Inches(5.0)
        set_cell_background(c1, "F8FAFC")
        set_cell_background(c2, "FFFFFF")
        set_cell_margins(c1, top=60, bottom=60, left=100, right=100)
        set_cell_margins(c2, top=60, bottom=60, left=100, right=100)
        
        p1 = c1.paragraphs[0]
        p1.paragraph_format.space_after = Pt(0)
        r1 = p1.add_run(k)
        set_font(r1, size=8.5, bold=True, color_rgb=NAVY_RGB)
        
        p2 = c2.paragraphs[0]
        p2.paragraph_format.space_after = Pt(0)
        r2 = p2.add_run(v)
        set_font(r2, size=8.5, color_rgb=SLATE_BODY_RGB)

    # ---------------- PAGE 2: TABLE OF CONTENTS ----------------
    doc.add_page_break()
    p_toc = doc.add_paragraph()
    p_toc.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_toc.paragraph_format.space_after = Pt(2)
    r = p_toc.add_run("Table of Contents")
    set_font(r, size=18, bold=True, color_rgb=NAVY_RGB)

    p_toc_sub = doc.add_paragraph()
    p_toc_sub.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p_toc_sub.paragraph_format.space_after = Pt(14)
    r = p_toc_sub.add_run("YP-Raksha v2.5 — Complete Enterprise Technical Architecture Index")
    set_font(r, size=9.5, italic=True, color_rgb=MUTED_RGB)

    toc_data = [
        ("1.0", "Product Overview & Mission", "1.1 Intro to Raksha | 1.2 Purpose & Objectives | 1.3 Key Features | 1.4 Use Cases & Benefits | 1.5 Scope & Limitations", "Page 3 - 4"),
        ("2.0", "System Architecture & High-Level Design", "2.1 Dual Data Planes | 2.2 Ingestion & Proxy Pipelines | 2.3 Persistence & Multi-Storage Fabric | 2.4 Cryptographic Boundaries", "Page 5"),
        ("3.0", "Browser Guard & Endpoint Fleet Mechanics", "3.1 Python mitmproxy Agent | 3.2 PAC Steering (18103/18195) | 3.3 Dynamic Content Parsing | 3.4 Triple Remediation Pipeline", "Page 6"),
        ("4.0", "Unified AI Gateway & Provider Fabric", "4.1 OpenAI-Compatible /v1/* Modality Engine | 4.2 28+ Provider Adapters | 4.3 Failover & Load Balancing | 4.4 Model Context Protocol", "Page 7"),
        ("5.0", "Governance, Budgets & Virtual Keys", "5.1 Cryptographic Virtual Keys | 5.2 Multi-Tier Budget Quotas | 5.3 TPM/RPM Throttling | 5.4 Chargebacks & Cost Attribution", "Page 8"),
        ("6.0", "Security Enforcement & Hazard Screening", "6.1 Inline PII & Regulated Data Masking | 6.2 CEL Policy Rules | 6.3 Secondary AI Guard Bot | 6.4 Jailbreak & Prompt Injection Defense", "Page 9"),
        ("7.0", "Identity Federation & Enterprise RBAC", "7.1 Multi-Tenant RBAC Matrix | 7.2 SCIM 2.0 Inbound Sync (Okta/Entra) | 7.3 Active Directory LDAP/LDAPS | 7.4 Session Security Policies", "Page 10"),
        ("8.0", "Observability, Telemetry & Audit Trails", "8.1 Real-Time Prompt/Search Logs | 8.2 OTel & Prometheus Exporters | 8.3 Immutable Audit Trails | 8.4 CISO Analytics Dashboard", "Page 11"),
        ("9.0", "Enterprise Deployment & Infrastructure", "9.1 Containerized Stack (Docker & Helm) | 9.2 Endpoint Packages (Inno Setup / Mac) | 9.3 Secrets Hardening | 9.4 High Availability", "Page 12"),
        ("10.0", "Technical Glossary & Reference Matrix", "10.1 Key Terminology & Definitions | 10.2 Network Port & Protocol Matrix | 10.3 Failover Protocol & Diagnostic Runbook", "Page 13"),
    ]

    t_toc = doc.add_table(rows=len(toc_data) + 1, cols=4)
    t_toc.alignment = WD_TABLE_ALIGNMENT.CENTER
    headers = ["Sec", "Module & Topic Specification", "Detailed Engineering Subsystems & Topics", "Page"]
    for i, h in enumerate(headers):
        cell = t_toc.rows[0].cells[i]
        set_cell_background(cell, NAVY_HEX)
        set_cell_margins(cell, top=80, bottom=80, left=80, right=80)
        p = cell.paragraphs[0]
        p.paragraph_format.space_after = Pt(0)
        if i in (0, 3):
            p.alignment = WD_ALIGN_PARAGRAPH.CENTER
        r = p.add_run(h)
        set_font(r, size=8.5, bold=True, color_rgb=RGBColor(0xFF, 0xFF, 0xFF))

    col_widths = [Inches(0.5), Inches(2.2), Inches(3.6), Inches(0.8)]
    for r_idx, (sec, title, desc, pg) in enumerate(toc_data):
        row = t_toc.rows[r_idx + 1]
        bg = "FFFFFF" if r_idx % 2 == 0 else "F8FAFC"
        for c_idx, val in enumerate([sec, title, desc, pg]):
            cell = row.cells[c_idx]
            cell.width = col_widths[c_idx]
            set_cell_background(cell, bg)
            set_cell_margins(cell, top=60, bottom=60, left=80, right=80)
            p = cell.paragraphs[0]
            p.paragraph_format.space_after = Pt(0)
            if c_idx == 0:
                p.alignment = WD_ALIGN_PARAGRAPH.CENTER
                r = p.add_run(val)
                set_font(r, size=8, bold=True, color_rgb=NAVY_RGB)
            elif c_idx == 1:
                r = p.add_run(val)
                set_font(r, size=8, bold=True, color_rgb=NAVY_RGB)
            elif c_idx == 2:
                r = p.add_run(val)
                set_font(r, size=7.5, color_rgb=SLATE_BODY_RGB)
            else:
                p.alignment = WD_ALIGN_PARAGRAPH.CENTER
                r = p.add_run(val)
                set_font(r, size=8, bold=True, color_rgb=GREEN_RGB)

    p_road = doc.add_paragraph()
    p_road.paragraph_format.space_before = Pt(14)
    p_road.paragraph_format.space_after = Pt(0)
    r = p_road.add_run("Architectural Scope Note: ")
    set_font(r, size=8, bold=True, color_rgb=TEAL_RGB)
    r2 = p_road.add_run(
        "This technical specification is organized modularly to provide complete verification depth across all operational layers of YP-Raksha v2.5. "
        "Each section delivers verified implementation facts from source code, operational binaries, and configuration schemas, serving as the official technical baseline for deployment and enterprise infosec compliance."
    )
    set_font(r2, size=8, italic=True, color_rgb=SLATE_BODY_RGB)

    # ---------------- PAGE 3: 1. PRODUCT OVERVIEW (PART 1) ----------------
    doc.add_page_break()
    p_h1 = doc.add_paragraph()
    p_h1.paragraph_format.space_after = Pt(4)
    r = p_h1.add_run("1. Product Overview")
    set_font(r, size=16, bold=True, color_rgb=NAVY_RGB)

    # 1.1
    p_h2 = doc.add_paragraph()
    p_h2.paragraph_format.space_before = Pt(6)
    p_h2.paragraph_format.space_after = Pt(2)
    r = p_h2.add_run("1.1 Introduction to Raksha AI Security")
    set_font(r, size=11, bold=True, color_rgb=TEAL_RGB)

    p1_1_items = [
        "Raksha AI Security is an enterprise-grade AI security and governance solution developed by YesPanchi to help organizations monitor, control, and secure the use of Artificial Intelligence (AI) applications and services.",
        "As organizations increasingly adopt AI tools such as ChatGPT, Claude, Gemini, DeepSeek, and other AI-powered platforms, protecting sensitive business information becomes essential. Employees may unintentionally submit confidential data, customer information, internal documents, or other sensitive content to AI applications.",
        "Raksha AI Security helps address these risks by providing centralized monitoring, security policy enforcement, data loss prevention (DLP), and AI usage governance. It enables organizations to establish security controls over AI interactions while maintaining visibility into how AI applications are used across the enterprise.",
        "The solution is designed to support enterprise security requirements, including controlled AI access, sensitive-data protection, risk monitoring, and administrative oversight. It can be deployed according to an organization's infrastructure and security requirements, including on-premises deployment where supported and configured."
    ]
    for text in p1_1_items:
        p = doc.add_paragraph()
        p.paragraph_format.space_after = Pt(4)
        p.paragraph_format.line_spacing = 1.15
        r = p.add_run(text)
        set_font(r, size=9, color_rgb=SLATE_BODY_RGB)

    # 1.2
    p_h2 = doc.add_paragraph()
    p_h2.paragraph_format.space_before = Pt(6)
    p_h2.paragraph_format.space_after = Pt(2)
    r = p_h2.add_run("1.2 Product Purpose and Objectives")
    set_font(r, size=11, bold=True, color_rgb=TEAL_RGB)

    p = doc.add_paragraph()
    p.paragraph_format.space_after = Pt(3)
    r = p.add_run("The primary purpose of Raksha AI Security is to enable organizations to use AI technologies securely while reducing the risk of confidential information exposure, unauthorized AI usage, and policy violations.")
    set_font(r, size=9, color_rgb=SLATE_BODY_RGB)

    p = doc.add_paragraph()
    p.paragraph_format.space_after = Pt(3)
    r = p.add_run("The key objectives are:")
    set_font(r, size=9, color_rgb=SLATE_BODY_RGB)

    objs_docx = [
        ("AI Usage Monitoring", "Provide visibility into AI application usage and relevant interactions across monitored endpoints and supported platforms."),
        ("Data Loss Prevention (DLP)", "Identify and help prevent sensitive information, such as confidential business data and personally identifiable information, from being submitted to AI applications in violation of configured policies."),
        ("Security Policy Enforcement", "Allow administrators to define and enforce organizational policies for AI application access, usage, and data protection."),
        ("Risk Detection and Prevention", "Identify potentially risky AI interactions and apply configured security actions, such as warnings, blocking, or logging, where supported."),
        ("Centralized Administration", "Provide a centralized interface for managing security configurations, monitoring activity, and reviewing relevant security events."),
        ("AI Governance and Accountability", "Support organizations in establishing consistent controls, maintaining audit visibility, and improving oversight of enterprise AI usage."),
        ("Enterprise Data Protection", "Support the implementation of organizational security and data-residency requirements through appropriate deployment architecture and configuration."),
    ]
    for idx, (title, desc) in enumerate(objs_docx, 1):
        p_b = doc.add_paragraph()
        p_b.paragraph_format.space_after = Pt(2)
        p_b.paragraph_format.left_indent = Inches(0.2)
        r1 = p_b.add_run(f"{idx}. {title}: ")
        set_font(r1, size=8.5, bold=True, color_rgb=NAVY_RGB)
        r2 = p_b.add_run(desc)
        set_font(r2, size=8.5, color_rgb=SLATE_BODY_RGB)

    # 1.3
    p_h2 = doc.add_paragraph()
    p_h2.paragraph_format.space_before = Pt(6)
    p_h2.paragraph_format.space_after = Pt(2)
    r = p_h2.add_run("1.3 Key Features and Capabilities")
    set_font(r, size=11, bold=True, color_rgb=TEAL_RGB)

    p = doc.add_paragraph()
    p.paragraph_format.space_after = Pt(3)
    r = p.add_run("Raksha AI Security provides a set of security and governance capabilities for enterprise AI environments.")
    set_font(r, size=9, color_rgb=SLATE_BODY_RGB)

    feats_docx = [
        ("1. AI Application Monitoring", "Monitors supported AI applications and interactions through configured monitoring components, providing visibility into relevant AI usage and security events."),
        ("2. Real-Time Data Loss Prevention", "Evaluates monitored content against configured security rules to identify sensitive information and apply appropriate protective actions."),
        ("3. Prompt Inspection and Risk Detection", "Analyzes AI prompts and supported interaction data to identify potential security risks, sensitive information, and policy violations."),
        ("4. Policy and Rule Management", "Enables administrators to configure security rules, monitoring policies, risk thresholds, and enforcement settings according to organizational requirements."),
        ("5. Website and Application Controls", "Supports configured website and AI application monitoring and access-control policies for supported environments."),
        ("6. Security Logs and Audit Visibility", "Provides relevant logs and event information to help administrators investigate policy violations, review AI usage, and understand detected risks."),
        ("7. Centralized Dashboard", "Provides a consolidated view of available monitoring data, security events, risk indicators, and system activity to support administrative oversight."),
        ("8. Enterprise Access and Governance", "Supports controlled administration of users, teams, customers, business units, and virtual keys, where applicable to the deployed Gateway configuration."),
        ("9. AI Gateway Governance", "Supports centralized management of AI model providers, routing configurations, usage limits, and budget controls within the Gateway component."),
        ("10. Enterprise Deployment Support", "Supports deployment and configuration according to organizational infrastructure requirements, including on-premises environments where supported."),
    ]
    t_f_docx = doc.add_table(rows=5, cols=2)
    t_f_docx.alignment = WD_TABLE_ALIGNMENT.CENTER
    for i in range(0, len(feats_docx), 2):
        row_idx = i // 2
        for col_idx, (f_title, f_desc) in enumerate(feats_docx[i:i+2]):
            cell = t_f_docx.rows[row_idx].cells[col_idx]
            cell.width = Inches(3.5)
            set_cell_background(cell, "F8FAFC")
            set_cell_margins(cell, top=60, bottom=60, left=80, right=80)
            p = cell.paragraphs[0]
            p.paragraph_format.space_after = Pt(0)
            r1 = p.add_run(f"{f_title}\n")
            set_font(r1, size=8.5, bold=True, color_rgb=NAVY_RGB)
            r2 = p.add_run(f_desc)
            set_font(r2, size=8, color_rgb=SLATE_BODY_RGB)

    p_note = doc.add_paragraph()
    p_note.paragraph_format.space_before = Pt(4)
    p_note.paragraph_format.space_after = Pt(6)
    r = p_note.add_run("*Note: Actual feature availability, enforcement behavior, supported applications, and deployment options depend on the installed product version, enabled modules, and validated configuration.")
    set_font(r, size=8, italic=True, color_rgb=MUTED_RGB)

    # ---------------- PAGE 4: 1. PRODUCT OVERVIEW (PART 2) ----------------
    doc.add_page_break()

    # 1.4
    p_h2 = doc.add_paragraph()
    p_h2.paragraph_format.space_before = Pt(4)
    p_h2.paragraph_format.space_after = Pt(2)
    r = p_h2.add_run("1.4 Use Cases and Business Benefits")
    set_font(r, size=11, bold=True, color_rgb=TEAL_RGB)

    p = doc.add_paragraph()
    p.paragraph_format.space_after = Pt(3)
    r = p.add_run("Raksha AI Security can support a range of enterprise security and governance scenarios.")
    set_font(r, size=9, color_rgb=SLATE_BODY_RGB)

    ucs_docx = [
        ("Use Case 1: Preventing Confidential Data Exposure", "Employees may accidentally enter confidential company information into public AI tools. Raksha can inspect monitored interactions against configured policies and apply the appropriate security response.", "Reduces the risk of unauthorized disclosure of sensitive business information."),
        ("Use Case 2: Enterprise AI Usage Monitoring", "Security and IT teams need visibility into how employees use supported AI applications.", "Improves visibility, security oversight, and understanding of AI usage across the organization."),
        ("Use Case 3: Enforcing Organizational AI Policies", "Organizations may restrict the use of specific AI applications or prohibit certain categories of sensitive information from being submitted to AI services.", "Helps enforce consistent AI usage policies and reduce policy violations."),
        ("Use Case 4: Security Incident Investigation", "When a potential AI-related security incident occurs, administrators need relevant event logs and monitoring information to investigate the incident.", "Supports incident analysis, security reviews, and audit preparation."),
        ("Use Case 5: Enterprise AI Gateway Governance", "Organizations using multiple AI providers and models may need centralized routing, access management, and budget controls.", "Helps improve governance, operational visibility, and control over configured AI usage and expenditure."),
        ("Use Case 6: Regulated and Sensitive Environments", "Organizations such as banks and financial institutions may require strict controls over sensitive information, infrastructure access, and data residency.", "Supports the implementation of internal security and compliance controls when the solution is deployed and validated against the organization's requirements."),
    ]
    t_uc_docx = doc.add_table(rows=3, cols=2)
    t_uc_docx.alignment = WD_TABLE_ALIGNMENT.CENTER
    for i in range(0, len(ucs_docx), 2):
        row_idx = i // 2
        for col_idx, (uc_t, uc_d, uc_b) in enumerate(ucs_docx[i:i+2]):
            cell = t_uc_docx.rows[row_idx].cells[col_idx]
            cell.width = Inches(3.5)
            set_cell_background(cell, "F8FAFC")
            set_cell_margins(cell, top=60, bottom=60, left=80, right=80)
            p = cell.paragraphs[0]
            p.paragraph_format.space_after = Pt(0)
            r1 = p.add_run(f"{uc_t}\n")
            set_font(r1, size=8.5, bold=True, color_rgb=NAVY_RGB)
            r2 = p.add_run(f"{uc_d}\n")
            set_font(r2, size=8, color_rgb=SLATE_BODY_RGB)
            r3 = p.add_run("Business benefit: ")
            set_font(r3, size=8, bold=True, color_rgb=GREEN_RGB)
            r4 = p.add_run(uc_b)
            set_font(r4, size=8, color_rgb=SLATE_BODY_RGB)

    p_ob_title = doc.add_paragraph()
    p_ob_title.paragraph_format.space_before = Pt(4)
    p_ob_title.paragraph_format.space_after = Pt(2)
    r = p_ob_title.add_run("Overall Business Benefits")
    set_font(r, size=9.5, bold=True, color_rgb=NAVY_RGB)

    obs_docx = [
        "Improved visibility into supported AI usage.",
        "Reduced risk of sensitive-data exposure.",
        "More consistent enforcement of AI security policies.",
        "Centralized monitoring and administrative control.",
        "Improved support for security investigations and audits.",
        "Better governance of AI providers, models, and configured budgets.",
        "Support for enterprise security and data-residency requirements.",
    ]
    for ob in obs_docx:
        p_ob = doc.add_paragraph(style="List Bullet")
        p_ob.paragraph_format.space_after = Pt(1.5)
        r = p_ob.add_run(ob)
        set_font(r, size=8.5, color_rgb=SLATE_BODY_RGB)

    # 1.5
    p_h2 = doc.add_paragraph()
    p_h2.paragraph_format.space_before = Pt(6)
    p_h2.paragraph_format.space_after = Pt(2)
    r = p_h2.add_run("1.5 Product Scope and Limitations")
    set_font(r, size=11, bold=True, color_rgb=TEAL_RGB)

    p_sc_title = doc.add_paragraph()
    p_sc_title.paragraph_format.space_before = Pt(2)
    p_sc_title.paragraph_format.space_after = Pt(2)
    r = p_sc_title.add_run("Product Scope")
    set_font(r, size=9.5, bold=True, color_rgb=NAVY_RGB)

    p = doc.add_paragraph()
    p.paragraph_format.space_after = Pt(2)
    r = p.add_run("Raksha AI Security is intended to provide security monitoring, data protection, and governance capabilities for supported AI applications and configured enterprise AI environments. The product scope includes:")
    set_font(r, size=9, color_rgb=SLATE_BODY_RGB)

    scopes_docx = [
        "Monitoring supported AI applications and interactions.",
        "Inspecting monitored prompts and content against configured security rules.",
        "Detecting potential sensitive-data exposure and policy violations.",
        "Applying supported security actions according to configured policies.",
        "Maintaining relevant security logs and monitoring information.",
        "Providing administrative controls and dashboard visibility.",
        "Supporting configured AI Gateway governance, including provider, model, routing, and budget management.",
        "Supporting enterprise deployment architectures based on validated product capabilities and infrastructure requirements.",
    ]
    for sc in scopes_docx:
        p_sc = doc.add_paragraph(style="List Bullet")
        p_sc.paragraph_format.space_after = Pt(1.5)
        r = p_sc.add_run(sc)
        set_font(r, size=8.5, color_rgb=SLATE_BODY_RGB)

    p_lim_title = doc.add_paragraph()
    p_lim_title.paragraph_format.space_before = Pt(4)
    p_lim_title.paragraph_format.space_after = Pt(2)
    r = p_lim_title.add_run("Product Limitations")
    set_font(r, size=9.5, bold=True, color_rgb=NAVY_RGB)

    limits_docx = [
        ("Supported Applications", "Monitoring and enforcement depend on the AI applications, websites, browsers, operating systems, and integration methods supported by the deployed version."),
        ("Monitoring Coverage", "Raksha can inspect only the interactions and data accessible to its configured monitoring components. It should not be assumed to monitor every application or every communication channel automatically."),
        ("Detection Accuracy", "Detection performance depends on the configured rules, inspection methods, supported content types, and detection models. False positives and false negatives may occur."),
        ("Endpoint Configuration", "Monitoring and enforcement may depend on successful installation, configuration, permissions, and operation of the relevant endpoint agent, extension, or other monitoring component."),
        ("Network and Security Restrictions", "Firewalls, proxies, antivirus software, endpoint protection tools, and organizational security policies may affect installation, connectivity, or operation."),
        ("Deployment Requirements", "On-premises deployment, high availability, disaster recovery, endpoint scalability, and offline operation must be validated against the actual product architecture and deployment configuration."),
        ("Data Residency", "Keeping all data, logs, and telemetry within a bank's infrastructure or within India requires appropriate architecture, network restrictions, storage configuration, and verification. This must be confirmed through deployment testing and network-flow validation."),
        ("Third-Party AI Services", "When users access external AI services, information submitted to those services may be processed according to the respective provider's policies and infrastructure. Raksha monitoring alone does not guarantee that external providers will not retain or process submitted information."),
        ("Compliance Responsibility", "Raksha can support organizational security and governance processes but does not independently guarantee regulatory compliance or certification."),
        ("Feature Availability", "Certain features may depend on the product edition, enabled modules, software version, licenses, or administrative configuration."),
    ]
    for idx, (lt, ld) in enumerate(limits_docx, 1):
        p_l = doc.add_paragraph()
        p_l.paragraph_format.space_after = Pt(2)
        p_l.paragraph_format.left_indent = Inches(0.2)
        r1 = p_l.add_run(f"{idx}. {lt}: ")
        set_font(r1, size=8.5, bold=True, color_rgb=NAVY_RGB)
        r2 = p_l.add_run(ld)
        set_font(r2, size=8.5, color_rgb=SLATE_BODY_RGB)

    p_concl = doc.add_paragraph()
    p_concl.paragraph_format.space_before = Pt(4)
    p_concl.paragraph_format.space_after = Pt(4)
    r = p_concl.add_run("The deployment team should validate the supported environments, security controls, data flows, performance, and recovery procedures before production rollout.")
    set_font(r, size=8, italic=True, color_rgb=MUTED_RGB)

    doc.save(str(DOCX_OUTPUT))
    print(f"Built DOCX: {DOCX_OUTPUT}")

def build_markdown():
    content = """# YES PANCHI — YP-RAKSHA
## Real-Time AI Knowledge Screening & Hazard Audit
### Technical Documentation — Version 2.5

---

### Document Information & Metadata

| Metadata Field | Specification Details |
| :--- | :--- |
| **Product Name** | YP-Raksha (YesPanchi Raksha AI Security Platform) |
| **Document Type** | Enterprise Architecture & Technical Specification Manual |
| **System Version** | Version 2.5 (Gateway Core Go 1.26.4 / Python 3.11+ mitmproxy Guard Agent 1.1.20) |
| **Security Classification** | Confidential — Enterprise Internal & Partner Technical Audit |
| **Target Audience** | Chief Information Security Officers (CISO), Enterprise Architects, SecOps, InfoSec Compliance Auditors |
| **Publishing Organization** | YesPanchi Group of Companies — Cyber Defense & Enterprise AI Division |
| **Release Date** | October 2026 |
| **Document Status** | Approved Enterprise Baseline Technical Specification |

---

## Table of Contents

| Section | Module & Topic Specification | Detailed Engineering Subsystems & Topics | Page |
| :---: | :--- | :--- | :---: |
| **1.0** | **Product Overview & Mission** | 1.1 Intro to Raksha \\| 1.2 Purpose & Objectives \\| 1.3 Key Features \\| 1.4 Use Cases & Benefits \\| 1.5 Scope & Limitations | **Page 3 - 4** |
| **2.0** | **System Architecture & High-Level Design** | 2.1 Dual Data Planes \\| 2.2 Ingestion & Proxy Pipelines \\| 2.3 Persistence & Multi-Storage Fabric \\| 2.4 Cryptographic Boundaries | **Page 5** |
| **3.0** | **Browser Guard & Endpoint Fleet Mechanics** | 3.1 Python mitmproxy Agent \\| 3.2 PAC Steering (18103/18195) \\| 3.3 Dynamic Content Parsing \\| 3.4 Triple Remediation Pipeline | **Page 6** |
| **4.0** | **Unified AI Gateway & Provider Fabric** | 4.1 OpenAI-Compatible `/v1/*` Modality Engine \\| 4.2 28+ Provider Adapters \\| 4.3 Failover & Load Balancing \\| 4.4 Model Context Protocol | **Page 7** |
| **5.0** | **Governance, Budgets & Virtual Keys** | 5.1 Cryptographic Virtual Keys \\| 5.2 Multi-Tier Budget Quotas \\| 5.3 TPM/RPM Throttling \\| 5.4 Chargebacks & Cost Attribution | **Page 8** |
| **6.0** | **Security Enforcement & Hazard Screening** | 6.1 Inline PII & Regulated Data Masking \\| 6.2 CEL Policy Rules \\| 6.3 Secondary AI Guard Bot \\| 6.4 Jailbreak & Prompt Injection Defense | **Page 9** |
| **7.0** | **Identity Federation & Enterprise RBAC** | 7.1 Multi-Tenant RBAC Matrix \\| 7.2 SCIM 2.0 Inbound Sync (Okta/Entra) \\| 7.3 Active Directory LDAP/LDAPS \\| 7.4 Session Security Policies | **Page 10** |
| **8.0** | **Observability, Telemetry & Audit Trails** | 8.1 Real-Time Prompt/Search Logs \\| 8.2 OTel & Prometheus Exporters \\| 8.3 Immutable Audit Trails \\| 8.4 CISO Analytics Dashboard | **Page 11** |
| **9.0** | **Enterprise Deployment & Infrastructure** | 9.1 Containerized Stack (Docker & Helm) \\| 9.2 Endpoint Packages (Inno Setup / Mac) \\| 9.3 Secrets Hardening \\| 9.4 High Availability | **Page 12** |
| **10.0** | **Technical Glossary & Reference Matrix** | 10.1 Key Terminology & Definitions \\| 10.2 Network Port & Protocol Matrix \\| 10.3 Failover Protocol & Diagnostic Runbook | **Page 13** |

> **Architectural Scope Note:** This technical specification is organized modularly to provide complete verification depth across all operational layers of YP-Raksha v2.5. Each section delivers verified implementation facts from source code, operational binaries, and configuration schemas, serving as the official technical baseline for deployment and enterprise infosec compliance.

---

# 1. Product Overview

## 1.1 Introduction to Raksha AI Security

Raksha AI Security is an enterprise-grade AI security and governance solution developed by YesPanchi to help organizations monitor, control, and secure the use of Artificial Intelligence (AI) applications and services.

As organizations increasingly adopt AI tools such as ChatGPT, Claude, Gemini, DeepSeek, and other AI-powered platforms, protecting sensitive business information becomes essential. Employees may unintentionally submit confidential data, customer information, internal documents, or other sensitive content to AI applications.

Raksha AI Security helps address these risks by providing centralized monitoring, security policy enforcement, data loss prevention (DLP), and AI usage governance. It enables organizations to establish security controls over AI interactions while maintaining visibility into how AI applications are used across the enterprise.

The solution is designed to support enterprise security requirements, including controlled AI access, sensitive-data protection, risk monitoring, and administrative oversight. It can be deployed according to an organization's infrastructure and security requirements, including on-premises deployment where supported and configured.

## 1.2 Product Purpose and Objectives

The primary purpose of Raksha AI Security is to enable organizations to use AI technologies securely while reducing the risk of confidential information exposure, unauthorized AI usage, and policy violations.

The key objectives are:

1. **AI Usage Monitoring:** Provide visibility into AI application usage and relevant interactions across monitored endpoints and supported platforms.

2. **Data Loss Prevention (DLP):** Identify and help prevent sensitive information, such as confidential business data and personally identifiable information, from being submitted to AI applications in violation of configured policies.

3. **Security Policy Enforcement:** Allow administrators to define and enforce organizational policies for AI application access, usage, and data protection.

4. **Risk Detection and Prevention:** Identify potentially risky AI interactions and apply configured security actions, such as warnings, blocking, or logging, where supported.

5. **Centralized Administration:** Provide a centralized interface for managing security configurations, monitoring activity, and reviewing relevant security events.

6. **AI Governance and Accountability:** Support organizations in establishing consistent controls, maintaining audit visibility, and improving oversight of enterprise AI usage.

7. **Enterprise Data Protection:** Support the implementation of organizational security and data-residency requirements through appropriate deployment architecture and configuration.

## 1.3 Key Features and Capabilities

Raksha AI Security provides a set of security and governance capabilities for enterprise AI environments.

### 1. AI Application Monitoring
Monitors supported AI applications and interactions through configured monitoring components, providing visibility into relevant AI usage and security events.

### 2. Real-Time Data Loss Prevention
Evaluates monitored content against configured security rules to identify sensitive information and apply appropriate protective actions.

### 3. Prompt Inspection and Risk Detection
Analyzes AI prompts and supported interaction data to identify potential security risks, sensitive information, and policy violations.

### 4. Policy and Rule Management
Enables administrators to configure security rules, monitoring policies, risk thresholds, and enforcement settings according to organizational requirements.

### 5. Website and Application Controls
Supports configured website and AI application monitoring and access-control policies for supported environments.

### 6. Security Logs and Audit Visibility
Provides relevant logs and event information to help administrators investigate policy violations, review AI usage, and understand detected risks.

### 7. Centralized Dashboard
Provides a consolidated view of available monitoring data, security events, risk indicators, and system activity to support administrative oversight.

### 8. Enterprise Access and Governance
Supports controlled administration of users, teams, customers, business units, and virtual keys, where applicable to the deployed Gateway configuration.

### 9. AI Gateway Governance
Supports centralized management of AI model providers, routing configurations, usage limits, and budget controls within the Gateway component.

### 10. Enterprise Deployment Support
Supports deployment and configuration according to organizational infrastructure requirements, including on-premises environments where supported.

*Note: Actual feature availability, enforcement behavior, supported applications, and deployment options depend on the installed product version, enabled modules, and validated configuration.*

## 1.4 Use Cases and Business Benefits

Raksha AI Security can support a range of enterprise security and governance scenarios.

### Use Case 1: Preventing Confidential Data Exposure
Employees may accidentally enter confidential company information into public AI tools. Raksha can inspect monitored interactions against configured policies and apply the appropriate security response.

**Business benefit:** Reduces the risk of unauthorized disclosure of sensitive business information.

### Use Case 2: Enterprise AI Usage Monitoring
Security and IT teams need visibility into how employees use supported AI applications.

**Business benefit:** Improves visibility, security oversight, and understanding of AI usage across the organization.

### Use Case 3: Enforcing Organizational AI Policies
Organizations may restrict the use of specific AI applications or prohibit certain categories of sensitive information from being submitted to AI services.

**Business benefit:** Helps enforce consistent AI usage policies and reduce policy violations.

### Use Case 4: Security Incident Investigation
When a potential AI-related security incident occurs, administrators need relevant event logs and monitoring information to investigate the incident.

**Business benefit:** Supports incident analysis, security reviews, and audit preparation.

### Use Case 5: Enterprise AI Gateway Governance
Organizations using multiple AI providers and models may need centralized routing, access management, and budget controls.

**Business benefit:** Helps improve governance, operational visibility, and control over configured AI usage and expenditure.

### Use Case 6: Regulated and Sensitive Environments
Organizations such as banks and financial institutions may require strict controls over sensitive information, infrastructure access, and data residency.

**Business benefit:** Supports the implementation of internal security and compliance controls when the solution is deployed and validated against the organization's requirements.

### Overall Business Benefits

- Improved visibility into supported AI usage.
- Reduced risk of sensitive-data exposure.
- More consistent enforcement of AI security policies.
- Centralized monitoring and administrative control.
- Improved support for security investigations and audits.
- Better governance of AI providers, models, and configured budgets.
- Support for enterprise security and data-residency requirements.

## 1.5 Product Scope and Limitations

### Product Scope

Raksha AI Security is intended to provide security monitoring, data protection, and governance capabilities for supported AI applications and configured enterprise AI environments.

The product scope includes:

- Monitoring supported AI applications and interactions.
- Inspecting monitored prompts and content against configured security rules.
- Detecting potential sensitive-data exposure and policy violations.
- Applying supported security actions according to configured policies.
- Maintaining relevant security logs and monitoring information.
- Providing administrative controls and dashboard visibility.
- Supporting configured AI Gateway governance, including provider, model, routing, and budget management.
- Supporting enterprise deployment architectures based on validated product capabilities and infrastructure requirements.

### Product Limitations

1. **Supported Applications:** Monitoring and enforcement depend on the AI applications, websites, browsers, operating systems, and integration methods supported by the deployed version.

2. **Monitoring Coverage:** Raksha can inspect only the interactions and data accessible to its configured monitoring components. It should not be assumed to monitor every application or every communication channel automatically.

3. **Detection Accuracy:** Detection performance depends on the configured rules, inspection methods, supported content types, and detection models. False positives and false negatives may occur.

4. **Endpoint Configuration:** Monitoring and enforcement may depend on successful installation, configuration, permissions, and operation of the relevant endpoint agent, extension, or other monitoring component.

5. **Network and Security Restrictions:** Firewalls, proxies, antivirus software, endpoint protection tools, and organizational security policies may affect installation, connectivity, or operation.

6. **Deployment Requirements:** On-premises deployment, high availability, disaster recovery, endpoint scalability, and offline operation must be validated against the actual product architecture and deployment configuration.

7. **Data Residency:** Keeping all data, logs, and telemetry within a bank's infrastructure or within India requires appropriate architecture, network restrictions, storage configuration, and verification. This must be confirmed through deployment testing and network-flow validation.

8. **Third-Party AI Services:** When users access external AI services, information submitted to those services may be processed according to the respective provider's policies and infrastructure. Raksha monitoring alone does not guarantee that external providers will not retain or process submitted information.

9. **Compliance Responsibility:** Raksha can support organizational security and governance processes but does not independently guarantee regulatory compliance or certification.

10. **Feature Availability:** Certain features may depend on the product edition, enabled modules, software version, licenses, or administrative configuration.

The deployment team should validate the supported environments, security controls, data flows, performance, and recovery procedures before production rollout.
"""
    with open(MD_OUTPUT, "w", encoding="utf-8") as f:
        f.write(content)
    print(f"Built Markdown: {MD_OUTPUT}")

if __name__ == "__main__":
    build_pdf()
    build_docx()
    build_markdown()
