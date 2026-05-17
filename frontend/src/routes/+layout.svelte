<script>
import "../app.css";
import favicon from '$lib/assets/favicon.svg';
import 'highlight.js/styles/atom-one-dark.css';
  
let { children } = $props();</script>

<svelte:head>
	<link rel="icon" href="{favicon}" />
</svelte:head>

{@render children()}
