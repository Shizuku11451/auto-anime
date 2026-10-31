package com.autoanime.android.app

import android.view.LayoutInflater
import android.view.ViewGroup
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.ListAdapter
import androidx.recyclerview.widget.RecyclerView
import com.autoanime.android.core.Anime
import com.autoanime.android.databinding.RowAnimeBinding

class AnimeAdapter(private val open: (Anime) -> Unit) : ListAdapter<Anime, AnimeAdapter.Holder>(DIFF) {
    init { setHasStableIds(true) }
    override fun getItemId(position: Int) = getItem(position).id
    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int) = Holder(RowAnimeBinding.inflate(LayoutInflater.from(parent.context), parent, false), open)
    override fun onBindViewHolder(holder: Holder, position: Int) = holder.bind(getItem(position))
    class Holder(private val binding: RowAnimeBinding, private val open: (Anime) -> Unit) : RecyclerView.ViewHolder(binding.root) {
        private var anime: Anime? = null
        init { binding.root.setOnClickListener { anime?.let(open) } }
        fun bind(value: Anime) { anime = value; binding.title.text = value.title; binding.subtitle.text = value.meta_name_cn.ifBlank { value.meta_name }.ifBlank { "Season ${value.season}" } }
    }
    companion object { private val DIFF = object : DiffUtil.ItemCallback<Anime>() {
        override fun areItemsTheSame(old: Anime, new: Anime) = old.id == new.id
        override fun areContentsTheSame(old: Anime, new: Anime) = old == new
    } }
}
