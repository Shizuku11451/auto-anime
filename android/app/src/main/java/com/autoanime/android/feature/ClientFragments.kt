package com.autoanime.android.feature

import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import androidx.fragment.app.Fragment
import androidx.fragment.app.activityViewModels
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import androidx.navigation.fragment.findNavController
import androidx.recyclerview.widget.LinearLayoutManager
import coil3.request.ImageRequest
import coil3.request.Disposable
import coil3.load
import com.autoanime.android.AutoAnimeApp
import com.autoanime.android.R
import com.autoanime.android.app.AnimeAdapter
import com.autoanime.android.core.MediaItem
import com.autoanime.android.core.ServerConnection
import com.autoanime.android.databinding.FragmentConnectBinding
import com.autoanime.android.databinding.FragmentDetailBinding
import com.autoanime.android.databinding.FragmentLibraryBinding
import com.autoanime.android.databinding.FragmentPlayerBinding
import kotlinx.coroutines.launch
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import androidx.media3.common.MediaItem as PlayerMediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.AudioAttributes
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.datasource.HttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.ProgressiveMediaSource

class ConnectFragment : Fragment() {
    private var _binding: FragmentConnectBinding? = null
    private val binding get() = _binding!!
    private val model: ClientViewModel by activityViewModels()
    private val app get() = requireActivity().application as AutoAnimeApp

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, state: Bundle?): View {
        _binding = FragmentConnectBinding.inflate(inflater, container, false)
        return binding.root
    }

    override fun onViewCreated(view: View, state: Bundle?) {
        viewLifecycleOwner.lifecycleScope.launch {
            val saved = withContext(Dispatchers.IO) { app.savedConnection() }
            if (saved != null && binding.serverUrl.text.isNullOrEmpty()) {
                binding.serverUrl.setText(saved.url)
                binding.allowHttp.isChecked = saved.allowHttp
                binding.username.setText(saved.account)
                model.restore(app, saved)
            }
        }
        binding.connect.setOnClickListener {
            try {
                val server = ServerConnection.parse(binding.serverUrl.text.toString(), binding.allowHttp.isChecked)
                val username = binding.username.text.toString()
                val password = binding.password.text.toString()
                binding.password.text?.clear()
                model.connect(app, server, username, password)
            } catch (e: Exception) { binding.status.text = e.message ?: "Invalid connection" }
        }
        viewLifecycleOwner.lifecycleScope.launch {
            viewLifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
                model.state.collect { state ->
                    binding.progress.visibility = if (state.loading) View.VISIBLE else View.GONE
                    binding.connect.isEnabled = !state.loading
                    binding.status.text = when {
                        state.setupRequired -> "Server setup is required. Complete setup in the browser first."
                        else -> state.error.orEmpty()
                    }
                    if (state.connected && findNavController().currentDestination?.id == R.id.connectFragment) {
                        findNavController().navigate(R.id.action_connect_to_library)
                    }
                }
            }
        }
    }

    override fun onDestroyView() { super.onDestroyView(); _binding = null }
}

class LibraryFragment : Fragment() {
    private var _binding: FragmentLibraryBinding? = null
    private val binding get() = _binding!!
    private val model: ClientViewModel by activityViewModels()

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, state: Bundle?): View {
        _binding = FragmentLibraryBinding.inflate(inflater, container, false)
        return binding.root
    }

    override fun onViewCreated(view: View, state: Bundle?) {
        val app = requireActivity().application as AutoAnimeApp
        if (!model.state.value.connected) {
            findNavController().navigate(R.id.action_library_logout)
            return
        }
        val adapter = AnimeAdapter { anime ->
            model.loadDetail(anime.id)
            findNavController().navigate(R.id.action_library_to_detail, Bundle().apply { putLong("animeId", anime.id) })
        }
        binding.animeList.layoutManager = LinearLayoutManager(requireContext())
        binding.animeList.adapter = adapter
        binding.loadMore.setOnClickListener { model.loadPage(append = true) }
        binding.logout.setOnClickListener {
            model.logout(requireActivity().application as AutoAnimeApp)
            findNavController().navigate(R.id.action_library_logout)
        }
        if (model.state.value.animes.isEmpty() && !model.state.value.loading) model.loadPage()
        viewLifecycleOwner.lifecycleScope.launch {
            viewLifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
                model.state.collect { state ->
                    if (state.requiresLogin) {
                        model.disconnectForExpiredSession(app)
                        findNavController().navigate(R.id.action_library_logout)
                        return@collect
                    }
                    adapter.submitList(state.animes)
                    binding.status.text = state.error.orEmpty().ifBlank { if (state.loading) "Loading…" else "" }
                    binding.loadMore.visibility = if (state.cursor != null) View.VISIBLE else View.GONE
                    binding.loadMore.isEnabled = !state.loading
                }
            }
        }
    }

    override fun onDestroyView() { binding.animeList.adapter = null; super.onDestroyView(); _binding = null }
}

class DetailFragment : Fragment() {
    private var _binding: FragmentDetailBinding? = null
    private val binding get() = _binding!!
    private val model: ClientViewModel by activityViewModels()
    private var requestedId = -1L
    private var coverRequest: Disposable? = null
    private var requestedCover = ""
    private val app get() = requireActivity().application as AutoAnimeApp

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, state: Bundle?): View {
        _binding = FragmentDetailBinding.inflate(inflater, container, false)
        return binding.root
    }

    override fun onViewCreated(view: View, state: Bundle?) {
        requestedId = requireArguments().getLong("animeId")
        if (!model.state.value.connected) {
            findNavController().navigate(R.id.action_detail_logout)
            return
        }
        if (model.state.value.detail?.id != requestedId) model.restoreDetail(requestedId)
        binding.back.setOnClickListener { findNavController().popBackStack() }
        binding.logout.setOnClickListener {
            model.logout(requireActivity().application as AutoAnimeApp)
            findNavController().navigate(R.id.action_detail_logout)
        }
        viewLifecycleOwner.lifecycleScope.launch {
            viewLifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
                model.state.collect { state ->
                    if (state.requiresLogin) {
                        model.disconnectForExpiredSession(app)
                        findNavController().navigate(R.id.action_detail_logout)
                        return@collect
                    }
                    val detail = state.detail?.takeIf { it.id == requestedId }
                    binding.status.text = state.error.orEmpty().ifBlank { if (detail == null || state.loading) "Loading…" else "" }
                    if (detail != null) render(detail.title, detail.cover, detail.summary, detail.seasons.flatMap { season ->
                        season.episodes.flatMap { it.items } + season.loose
                    }.filter { it.status == "已保存" })
                }
            }
        }
    }

    private fun render(title: String, cover: String, summary: String, items: List<MediaItem>) {
        binding.title.text = title
        binding.summary.text = summary
        val server = model.connection()
        if (server != null && cover.isNotBlank()) {
            val url = server.resolve(cover.removePrefix("/"))
            val cacheKey = "${server.base}:${model.state.value.sessionCacheKey}:$url"
            if (requestedCover != cacheKey) {
                requestedCover = cacheKey
                coverRequest?.dispose()
                coverRequest = binding.cover.load(url, app.imageLoader) { memoryCacheKey(cacheKey) }
            }
        } else {
            requestedCover = ""
            coverRequest?.dispose()
            coverRequest = null
            binding.cover.setImageDrawable(null)
        }
        val container = binding.items
        container.removeAllViews()
        items.forEach { item ->
            container.addView(Button(requireContext()).apply {
                tag = item.id
                text = item.title.ifBlank { "Episode ${item.episode}" }
                setOnClickListener {
                    model.setPlayback(item.id)
                    findNavController().navigate(R.id.action_detail_to_player)
                }
            })
        }
    }

    override fun onDestroyView() {
        coverRequest?.dispose(); coverRequest = null; requestedCover = ""
        if (findNavController().currentDestination?.id != R.id.playerFragment) model.clearDetail(requestedId)
        super.onDestroyView(); _binding = null
    }
}

@androidx.media3.common.util.UnstableApi
class PlayerFragment : Fragment() {
    private var _binding: FragmentPlayerBinding? = null
    private val binding get() = _binding!!
    private val model: ClientViewModel by activityViewModels()
    private var player: ExoPlayer? = null
    private var playbackSession = -1L

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, state: Bundle?): View {
        _binding = FragmentPlayerBinding.inflate(inflater, container, false)
        return binding.root
    }

    override fun onViewCreated(view: View, state: Bundle?) {
        binding.close.setOnClickListener { player?.release(); player = null; model.setPlayback(null); findNavController().popBackStack() }
        startPlayback()
    }

    override fun onStart() {
        super.onStart()
        if (player == null) startPlayback()
    }

    private fun startPlayback() {
        val current = _binding ?: return
        val app = requireActivity().application as AutoAnimeApp
        val item = model.state.value.detail?.seasons?.flatMap { season -> season.episodes.flatMap { it.items } + season.loose }
            ?.firstOrNull { it.id == model.state.value.playbackItemId && it.status == "已保存" }
        val server = model.connection()
        val client = app.streamHttp
        if (item == null || server == null || client == null) {
            current.error.setText(R.string.video_unavailable)
            return
        }
        try {
            val session = model.state.value.sessionCacheKey
            if (playbackSession != session) {
                model.savePlayback(0, true)
                playbackSession = session
            }
            val exo = ExoPlayer.Builder(requireContext()).build()
            player = exo
            val position = model.state.value.playbackPosition
            val playWhenReady = model.state.value.playbackWhenReady
            current.playerView.player = exo
            exo.setAudioAttributes(AudioAttributes.Builder().setUsage(androidx.media3.common.C.USAGE_MEDIA).setContentType(androidx.media3.common.C.AUDIO_CONTENT_TYPE_MOVIE).build(), true)
            exo.setHandleAudioBecomingNoisy(true)
            exo.addListener(object : Player.Listener {
                override fun onPlayerError(error: PlaybackException) {
                    val status = generateSequence<Throwable>(error) { it.cause }
                        .filterIsInstance<HttpDataSource.InvalidResponseCodeException>().firstOrNull()?.responseCode
                    if (status == 401) {
                        model.disconnectForExpiredSession(app)
                        _binding?.error?.setText(R.string.session_expired)
                        findNavController().navigate(R.id.action_player_logout)
                    } else _binding?.error?.setText(playbackErrorMessage(status, error))
                }
            })
            val source = ProgressiveMediaSource.Factory(OkHttpDataSource.Factory(client))
                .createMediaSource(PlayerMediaItem.fromUri(server.resolve("api/items/${item.id}/stream").toString()))
            exo.setMediaSource(source, position)
            exo.prepare()
            exo.playWhenReady = playWhenReady
        } catch (_: Exception) { current.error.setText(R.string.playback_start_failed) }
    }

    override fun onStop() {
        super.onStop()
        player?.let { model.savePlayback(it.currentPosition, it.playWhenReady) }
        _binding?.playerView?.player = null
        player?.release(); player = null
    }

    override fun onDestroyView() {
        binding.playerView.player = null
        player?.release(); player = null
        super.onDestroyView(); _binding = null
    }
}

fun playbackErrorMessage(status: Int?, error: PlaybackException): Int = when {
    status != null -> R.string.playback_http_failed
    error.errorCode == PlaybackException.ERROR_CODE_DECODING_FAILED || error.errorCode == PlaybackException.ERROR_CODE_DECODING_FORMAT_UNSUPPORTED -> R.string.playback_format_failed
    else -> R.string.playback_failed
}
