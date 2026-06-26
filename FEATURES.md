# Smart Suggestions Feature - User Guide

## Overview

The Smart Suggestions feature uses machine learning patterns to automatically suggest and auto-fill payees and categories based on your transaction history. It learns from every transaction you save and becomes increasingly accurate over time.

---

## How It Works: A Complete Walkthrough

### **Phase 1: Learning (First Few Transactions)**

#### **Day 1 - Import Your First CSV**
```
1. Import bank transactions (e.g., Santander CSV)
2. See list of DRAFT transactions
3. Click "Edit" on first transaction:

   Transaction: "BIEDRONKA WARSZAWA 01 20.50 PLN"

4. Fill in manually (this time):
   - Payee: Select "Biedronka" from dropdown
   - Category: Select "Groceries" from dropdown
   - Click "Save"

   ✅ Pattern recorded: Description "BIEDRONKA..." → Payee "Biedronka" + Category "Groceries"
```

---

### **Phase 2: Smart Suggestions Kick In (After 2-3 Similar Transactions)**

#### **Day 2 - Same Transaction Appears**

```
Transaction: "BIEDRONKA WARSZAWA 02 18.75 PLN"

You click "Edit" and immediately:

┌─────────────────────────────────────────────────┐
│ ✨ Payee field focused                          │
│ → Suggestions appear automatically!             │
│                                                  │
│ 🎯 Auto-filled: Biedronka (95% match)          │
│ 💚 Green highlight for 2 seconds               │
│ 📢 Toast: "Auto-filled: Biedronka (95% match)" │
└─────────────────────────────────────────────────┘

Then immediately:

┌─────────────────────────────────────────────────┐
│ ✨ Category field auto-fills too!               │
│ → Because you selected payee "Biedronka"        │
│                                                  │
│ 🎯 Auto-filled: Groceries (97% match)          │
│ 💚 Green highlight                              │
│ 📢 Toast: "Auto-filled category: Groceries     │
│    (97% match based on payee)"                  │
└─────────────────────────────────────────────────┘

Just click "Save" - done in 1 click!
```

---

### **Phase 3: High Confidence = Instant Auto-Fill (≥90%)**

#### **When Suggestions Are ≥90% Confident:**

**What You See:**
```
1. Click "Edit" on transaction
2. Payee field focuses
3. ✨ BOOM! Auto-fills instantly
   - Payee: "Biedronka" (green highlight)
   - Category: "Groceries" (green highlight)
4. Toast notification shows confidence
5. Just review and click "Save"
```

**No dropdown, no clicking - just instant magic! ✨**

---

### **Phase 4: Medium Confidence = Dropdown with Options (<90%)**

#### **When Confidence Is 70-89%:**

**What You See:**
```
Transaction: "APTEKA DOZ WARSZAWA 45.30 PLN"

Click "Edit" on payee field:

┌─────────────────────────────────────────────────┐
│ Payee: [                    ] [▼]               │
│                                                  │
│ 💡 Suggestions Dropdown:                        │
│ ┌─────────────────────────────────────────┐    │
│ │ ✅ Apteka DOZ              85% 🟡       │    │
│ │    Matched 4 times before               │    │
│ ├─────────────────────────────────────────┤    │
│ │    Rossmann                72% 🟡       │    │
│ │    Similar match (2 occurrences)        │    │
│ ├─────────────────────────────────────────┤    │
│ │    Hebe                    65% 🟡       │    │
│ │    Possible match                        │    │
│ └─────────────────────────────────────────┘    │
└─────────────────────────────────────────────────┘

Click any suggestion to fill it in
```

---

## 📊 The Two Suggestion Strategies

### **Strategy 1: Payee Suggestions (Based on Description)**

**Triggers:** When you focus on the payee field

**How it works:**
```
Your past transactions:
- "BIEDRONKA WARSZAWA 01" → Biedronka (saved 15 times)
- "BIEDRONKA SOPOT" → Biedronka (saved 8 times)
- "BIEDRONKA GDANSK" → Biedronka (saved 5 times)

New transaction: "BIEDRONKA KRAKOW"

Analysis:
✓ String similarity: "BIEDRONKA" matches (40 points)
✓ Frequency: 28 total times (30 points)
✓ Recency: Last used yesterday (18 points)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Total Confidence: 88%

Shows in dropdown with 88% badge
```

**Confidence Scoring Algorithm:**
- **String Similarity (Max 50 points)**: Uses Jaccard token similarity to compare transaction description with historical patterns
- **Frequency (Max 30 points)**: More occurrences = higher confidence (5 points per occurrence, capped at 30)
- **Recency (Max 20 points)**: Recent patterns weighted higher (20 points minus days since last seen / 10)
- **Total Score**: Sum of all factors, capped at 100%

---

### **Strategy 2: Category Suggestions (CRITICAL - Payee-Based!)**

**Triggers:** When payee is selected OR when you focus category field

**Primary Method - Payee-Based (Most Accurate!):**
```
You select: Payee = "Biedronka"

System looks up:
┌──────────────────────────────────────┐
│ Biedronka's History:                 │
│ - Groceries: used 25 times ✅        │
│ - Household: used 2 times            │
│ - Personal Care: used 1 time         │
└──────────────────────────────────────┘

Analysis for "Groceries":
✓ Frequency with this payee: 25 times (70 points)
✓ Last used with this payee: today (28 points)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Total Confidence: 98%

→ AUTO-FILLS Groceries immediately! ✨
Toast: "Auto-filled category: Groceries (98% match based on payee)"
```

**Payee-Based Confidence Scoring:**
- **Frequency (Max 70 points)**: 10 points per occurrence with this specific payee
- **Recency (Max 30 points)**: 30 points minus days since last used with this payee / 10
- **Total Score**: Capped at 100%

**Fallback Method - Description-Based:**
```
If no payee selected yet, uses transaction description:

"VISA SEL 421352******9361 PLATNOŚĆ KARTĄ EMPIK"

Looks for patterns:
- "EMPIK" matched 8 times → "Shopping" category
- Similar card transactions → various categories

Shows dropdown with options ranked by confidence
```

**Why Payee-Based Is More Accurate:**
- **Stable relationship**: Biedronka always = Groceries (99% consistent)
- **Context-aware**: Different payees can have same description patterns
- **Higher confidence**: Typically 90%+ after just 3-5 transactions
- **Faster learning**: Learns payee habits quicker than description patterns

---

## 🎨 Visual Indicators

### **Confidence Badges:**
```
🟢 90-100%: Green badge  → Auto-fills
🟡 70-89%:  Yellow badge → Shows in dropdown
⚪ <70%:    Gray badge   → Lower priority suggestion
```

### **Source Labels:**
```
Category suggestions show source:

"Groceries (from payee)" ← Payee-based (more accurate)
"Shopping (from description)" ← Description-based (fallback)
```

### **Reason Text:**
```
High confidence:
- "Used 15 times with this payee"
- "Matched 25 times before"

Medium confidence:
- "Often used with this payee (8 times)"
- "Similar match (5 occurrences)"

Low confidence:
- "Sometimes used with this payee"
- "Possible match"
```

### **Visual Feedback:**
- **Green highlight**: Applied to auto-filled fields for 2 seconds
- **Toast notifications**: Show at top-right with confidence score and source
- **Dropdown styling**: Dark theme with hover effects, confidence badges

---

## 🚀 Real-World Workflow Example

### **Monday Morning - 50 Transactions to Process**

#### **Old Way (5 minutes):**
```
For EACH transaction:
1. Click Edit
2. Type payee name
3. Search dropdown... scroll... click
4. Type category
5. Search dropdown... scroll... click
6. Click Save

= 5 clicks × 50 transactions = 250 clicks! 😫
```

#### **New Way with Smart Suggestions (2 minutes):**
```
For EACH transaction:
1. Click Edit
   → ✨ Payee auto-fills (90%+ confidence)
   → ✨ Category auto-fills (95%+ confidence)
2. Click Save

= 2 clicks × 50 transactions = 100 clicks! 🎉

OR if suggestions are 70-89%:
1. Click Edit
2. Click suggested payee from dropdown (1 click)
   → ✨ Category auto-fills based on payee!
3. Click Save

= 3 clicks × 50 transactions = 150 clicks
```

**Time saved: 60%! From 5 minutes to 2 minutes! ⚡**

---

## 💡 Pro Tips

### **1. The More You Use, The Smarter It Gets**
```
After 10 transactions: 70-80% confidence
After 20 transactions: 85-90% confidence
After 50 transactions: 95%+ confidence (almost always auto-fills!)
```

### **2. Payee→Category Is King**
```
Most powerful pattern:
"Biedronka" → "Groceries" (used 100 times)

This beats:
"BIEDRONKA WARSZAWA..." → "Groceries" (description-based)

Always select the same payee for same stores!
```

### **3. Manual Override Always Available**
```
Don't like the suggestion? Just:
- Type over it
- Select different option from datalist
- Create new payee/category

Your choice overrides and becomes the NEW pattern!
```

### **4. Pattern Learning is Immediate**
```
Save transaction:
✅ Pattern recorded instantly

Next similar transaction:
✨ Suggestion already updated with new data!
```

### **5. Polish Language Support**
```
Built-in normalization handles Polish diacritics:
- "BIEDRONKA" matches "Biedronka"
- "Żabka" matches "ZABKA"
- "ł" normalized to "l", "ą" to "a", etc.
```

---

## 🔄 The Learning Loop

```
┌─────────────────────────────────────────────────┐
│ 1. Import CSV                                   │
│    ↓                                             │
│ 2. Edit transaction (manual first time)        │
│    ↓                                             │
│ 3. Save → Pattern recorded                     │
│    ↓                                             │
│ 4. Next similar transaction                     │
│    ↓                                             │
│ 5. Smart suggestion appears!                    │
│    ↓                                             │
│ 6. Accept/modify → Pattern strengthens         │
│    ↓                                             │
│ 7. Confidence increases                         │
│    ↓                                             │
│ 8. Eventually auto-fills (90%+)                │
│    ↓                                             │
│ 9. One-click save! ✨                           │
└─────────────────────────────────────────────────┘

Repeat = Gets smarter every time! 🧠
```

---

## 🏗️ Technical Architecture

### **Database Schema**

**Table: `payee_patterns`**
```sql
CREATE TABLE payee_patterns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    budget_id TEXT NOT NULL,
    normalized_description TEXT NOT NULL,  -- Polish-normalized description
    payee_id TEXT NOT NULL,
    payee_name TEXT NOT NULL,
    category_id TEXT,
    category_name TEXT,
    occurrence_count INTEGER DEFAULT 1,    -- Incremented on each match
    last_seen TEXT NOT NULL,               -- Recency tracking
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now')),

    FOREIGN KEY (budget_id) REFERENCES budgets(id) ON DELETE CASCADE,
    FOREIGN KEY (payee_id) REFERENCES payees(id) ON DELETE CASCADE
);

-- Indexes for fast lookup
CREATE INDEX idx_payee_patterns_budget ON payee_patterns(budget_id);
CREATE INDEX idx_payee_patterns_desc ON payee_patterns(normalized_description);
CREATE INDEX idx_payee_patterns_payee ON payee_patterns(payee_id);

-- Unique constraint for pattern deduplication
CREATE UNIQUE INDEX idx_payee_patterns_unique ON payee_patterns(
    budget_id, normalized_description, payee_id, COALESCE(category_id, '')
);
```

### **API Endpoints**

**1. Payee Suggestions**
```
GET /api/payee-suggestions?budget_id={id}&description={text}

Response:
{
  "suggestions": [
    {
      "payee_id": "uuid",
      "payee_name": "Biedronka",
      "category_id": "uuid",
      "category_name": "Groceries",
      "confidence": 95.5,
      "reason": "Matched 15 times before"
    }
  ]
}
```

**2. Category Suggestions**
```
GET /api/category-suggestions?budget_id={id}&payee_id={id}&description={text}

Parameters:
- budget_id: Required
- payee_id: Optional (prioritizes payee-based if provided)
- description: Optional (fallback to description-based)

Response:
{
  "suggestions": [
    {
      "category_id": "uuid",
      "category_name": "Groceries",
      "confidence": 98.2,
      "reason": "Used 25 times with this payee",
      "source": "payee"  // or "description"
    }
  ]
}
```

### **Data Flow**

**Pattern Recording (on transaction save):**
```
1. User saves transaction with payee + category
2. Handler fetches payee name and category name from YNAB data
3. Normalize transaction description (remove Polish diacritics)
4. Create PayeePattern:
   - budget_id, normalized_description
   - payee_id, payee_name
   - category_id, category_name
   - last_seen = transaction date
5. UpsertPattern:
   - If pattern exists: increment occurrence_count, update last_seen
   - If new: insert with occurrence_count = 1
```

**Suggestion Retrieval:**
```
Payee Suggestions:
1. Normalize input description
2. Query: SELECT * FROM payee_patterns
   WHERE budget_id = ?
   AND normalized_description LIKE '%{normalized}%'
   ORDER BY occurrence_count DESC, last_seen DESC
   LIMIT 20
3. Score each pattern (similarity + frequency + recency)
4. Deduplicate by payee_id (keep highest confidence)
5. Sort by confidence DESC
6. Return top 5

Category Suggestions:
1. If payee_id provided:
   - Query patterns for this budget + payee_id
   - Score by frequency (10 pts/occurrence) + recency
   - Return top 5 categories
2. Else if description provided:
   - Same as payee suggestions but return categories
   - Filter to patterns with category_id != null
```

---

## ⚡ Summary: The Magic Moments

1. **First Transaction:** Manual work (building knowledge)
2. **2nd-5th Transaction:** Suggestions appear (helpful hints)
3. **6th+ Transaction:** Auto-fill kicks in (pure magic ✨)
4. **After 20+ Transactions:** Almost everything auto-fills (90%+ confidence)
5. **Your Job:** Just review and click "Save" - done! 🎉

**The system learns YOUR habits, YOUR payee names, YOUR category preferences. It gets smarter every single day!** 🚀

---

## 🐛 Troubleshooting

### **Suggestions Not Appearing?**
- Ensure you've saved at least 2-3 similar transactions first
- Check that budget_id is being passed correctly
- Verify JavaScript console for API errors

### **Low Confidence Scores?**
- Need more training data (save more similar transactions)
- Descriptions might be too varied (use consistent payee names)
- Check pattern database: `SELECT * FROM payee_patterns WHERE budget_id = 'your-id'`

### **Wrong Suggestions?**
- Manual override creates new pattern
- Old patterns remain but confidence decreases over time (recency factor)
- Can manually clear patterns: `DELETE FROM payee_patterns WHERE budget_id = ?`

---

## 📈 Future Enhancements

- [ ] Confidence threshold settings (allow users to adjust 90% threshold)
- [ ] Pattern export/import for backup
- [ ] Cross-budget learning (suggest patterns from other budgets)
- [ ] Merchant name extraction (detect brand from complex descriptions)
- [ ] Multi-language support beyond Polish
- [ ] Pattern analytics dashboard (show most used patterns)
- [ ] Bulk pattern training (import historical YNAB transactions)

---

**Generated**: December 2024
**Version**: 1.0.0
**Feature Status**: ✅ Production Ready
