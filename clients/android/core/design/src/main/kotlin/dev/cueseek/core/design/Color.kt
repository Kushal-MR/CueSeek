package dev.cueseek.core.design

import androidx.compose.ui.graphics.Color

// Placeholder palette, carried over from the project template so the app has a theme to
// run under. P4 replaces it with CueSeek's token layer, whose highest-value output is a
// single status language: what healthy / degraded / unreachable / unknown look like, with
// a redundant non-colour encoding, holding up in dark mode and under dynamic colour.
// Status colours are not themeable — they carry meaning (ADR-0010).
val Purple80 = Color(0xFFD0BCFF)
val PurpleGrey80 = Color(0xFFCCC2DC)
val Pink80 = Color(0xFFEFB8C8)

val Purple40 = Color(0xFF6650A4)
val PurpleGrey40 = Color(0xFF625B71)
val Pink40 = Color(0xFF7D5260)
